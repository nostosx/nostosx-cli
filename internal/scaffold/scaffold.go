// Package scaffold renders embedded application templates without running external commands.
package scaffold

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/nostosx/nostosx-cli/internal/projectfs"
)

//go:embed all:templates
var templates embed.FS

// Options describes a new single-module application and its process topology.
type Options struct {
	Name, Module, Layout, Output string
	Transport                    string
	Agent                        bool
	Infrastructure               []string
}

// HTTP and GRPC select the transports included in the generated process.
func (o Options) HTTP() bool       { return o.Transport != "grpc" }
func (o Options) GRPC() bool       { return strings.Contains(o.Transport, "grpc") }
func (o Options) NeedsToken() bool { return o.GRPC() || o.Agent }

func normalizeTransport(value string) (string, error) {
	switch value {
	case "", "http":
		return "http", nil
	case "grpc":
		return "grpc", nil
	case "http,grpc", "grpc,http":
		return "http,grpc", nil
	default:
		return "", errors.New("transport must be http, grpc or http,grpc")
	}
}

type data struct {
	Options
	Context     string
	Service     string
	ServiceType string
	API         apiData
}

var projectName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
var packageName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

func validateName(name string, pkg bool) error {
	pattern := projectName
	if pkg {
		pattern = packageName
	}
	reserved := []string{"con", "prn", "aux", "nul", "clock", "main", "internal", "vendor", "testdata"}
	if len(name) > 63 || !pattern.MatchString(name) || token.Lookup(name).IsKeyword() || slices.Contains(reserved, name) ||
		(len(name) == 4 && (strings.HasPrefix(name, "com") || strings.HasPrefix(name, "lpt")) && name[3] >= '0' && name[3] <= '9') {
		return fmt.Errorf("invalid name %q: use a lowercase %s (at most 63 characters), excluding Go keywords and reserved names", name, map[bool]string{true: "Go package name", false: "project slug"}[pkg])
	}
	return nil
}

func (o Options) validate() error {
	if err := validateName(o.Name, false); err != nil {
		return err
	}
	if err := validateModule(o.Module); err != nil {
		return err
	}
	if o.Layout != "monolith" && o.Layout != "microservices" {
		return errors.New("layout must be monolith or microservices")
	}
	return nil
}

func validateModule(name string) error {
	if err := module.CheckPath(name); err != nil {
		return fmt.Errorf("module: %w", err)
	}
	if name != strings.ToLower(name) || strings.ContainsAny(name, "\\\n\r\t\"`[]") {
		return errors.New("module must be a lowercase Go module path")
	}
	return nil
}

// New creates Output (or Name when Output is empty). It refuses existing destinations,
// including empty directories and symlinks. Generation is offline and does not initialize Git.
func New(ctx context.Context, o Options) (string, error) {
	var err error
	o.Transport, err = normalizeTransport(o.Transport)
	if err != nil {
		return "", err
	}
	if err := o.validate(); err != nil {
		return "", err
	}
	o.Infrastructure, err = normalizeInfrastructure(o.Infrastructure)
	if err != nil {
		return "", err
	}
	files, err := render("project", data{Options: o})
	if err != nil {
		return "", err
	}
	if err := addInfrastructureFiles(files, o); err != nil {
		return "", err
	}
	target := o.Output
	if target == "" {
		target = o.Name
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// The parent must already exist. Mkdir is the exclusive claim on the destination.
	if err := os.Mkdir(target, 0o700); err != nil {
		return "", fmt.Errorf("create destination: %w", err)
	}
	root, err := os.OpenRoot(target)
	if err != nil {
		return "", errors.Join(err, os.Remove(target))
	}
	writeErr := writeFiles(ctx, root, files)
	if err := errors.Join(writeErr, root.Close()); err != nil {
		return "", errors.Join(err, os.Remove(target))
	}
	return target, nil
}

// Add creates a context or a standalone service in a generated project. It never
// edits existing application files; monolith assembly remains an explicit code change.
func Add(ctx context.Context, dir, kind, name string) (err error) {
	if kind != "context" && kind != "service" {
		return errors.New("kind must be context or service")
	}
	if err := validateName(name, true); err != nil {
		return err
	}
	contextName := name
	root, release, err := projectfs.Lock(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	modulePath, err := readModulePath(root)
	if err != nil {
		return err
	}
	if kind == "service" {
		if _, err := root.Stat("application/" + contextName + "/doc.go"); err != nil {
			return fmt.Errorf("service requires a context created with 'add context %s': %w", contextName, err)
		}
	}
	o := Options{Module: modulePath}
	if kind == "service" {
		o.Transport, err = projectTransport(root)
		if err != nil {
			return err
		}
		o.Infrastructure, err = projectInfrastructure(root)
		if err != nil {
			return err
		}
	}
	files, err := render(kind, data{Options: o, Context: contextName, Service: name, ServiceType: strings.ToUpper(name[:1]) + name[1:]})
	if err != nil {
		return err
	}
	return writeFiles(ctx, root, files)
}

// Read the generated assembly shape instead of persisting duplicate project metadata.
func projectTransport(root *os.Root) (string, error) {
	b, _, err := readAPIFile(root, "infrastructure/server/server.go")
	if err != nil {
		return "", fmt.Errorf("read project transports: %w", err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", b, 0)
	if err != nil {
		return "", fmt.Errorf("parse project transports: %w", err)
	}
	var http, grpc bool
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typ, ok := spec.(*ast.TypeSpec)
			if !ok || typ.Name.Name != "Servers" {
				continue
			}
			structure, ok := typ.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					http = http || name.Name == "HTTP"
					grpc = grpc || name.Name == "GRPC"
				}
			}
		}
	}
	switch {
	case http && grpc:
		return "http,grpc", nil
	case http:
		return "http", nil
	case grpc:
		return "grpc", nil
	default:
		return "", errors.New("server.Servers must declare HTTP or GRPC; select project transports explicitly")
	}
}

func readModulePath(root *os.Root) (string, error) {
	info, err := root.Lstat("go.mod")
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("go.mod must be a regular file, not a symlink")
	}
	f, err := root.Open("go.mod")
	if err != nil {
		return "", fmt.Errorf("open project go.mod: %w", err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return "", err
	}
	const maxSize = 1 << 20
	if !info.Mode().IsRegular() || info.Size() > maxSize {
		return "", errors.New("go.mod must be a regular file of at most 1 MiB")
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxSize {
		return "", errors.New("go.mod exceeds 1 MiB")
	}
	parsed, err := modfile.Parse("go.mod", b, nil)
	if err != nil {
		return "", fmt.Errorf("parse project go.mod: %w", err)
	}
	if parsed.Module == nil {
		return "", errors.New("go.mod must declare a module path")
	}
	name := parsed.Module.Mod.Path
	if err := validateModule(name); err != nil {
		return "", err
	}
	return name, nil
}

func render(group string, d data) (map[string][]byte, error) {
	files := make(map[string][]byte)
	prefix := "templates/" + group
	err := fs.WalkDir(templates, prefix, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(name, prefix+"/"), ".tmpl")
		if group == "project" {
			if !d.GRPC() && (strings.HasPrefix(rel, "infrastructure/rpc/") || rel == "buf.yaml" || strings.HasPrefix(rel, "api/")) {
				return nil
			}
			if !d.Agent && strings.HasPrefix(rel, "infrastructure/agent/") {
				return nil
			}
			if d.Layout == "monolith" && strings.HasPrefix(rel, "services/") && strings.Contains(rel, "/cmd/") {
				return nil
			}
			if d.Layout == "microservices" && (rel == "app.go" || strings.HasPrefix(rel, "cmd/server/")) {
				return nil
			}
		}
		if group == "service" {
			if !d.GRPC() && (strings.HasPrefix(rel, "api/") || strings.Contains(rel, "/internal/port/rpc/")) {
				return nil
			}
			if !d.HTTP() && strings.Contains(rel, "/internal/port/http/") {
				return nil
			}
		}
		rel = strings.NewReplacer("__context__", d.Context, "__service__", d.Service, "__api__", d.API.File).Replace(rel)
		b, err := templates.ReadFile(name)
		if err != nil {
			return err
		}
		t, err := template.New(name).Delims("[[", "]]").Option("missingkey=error").Parse(string(b))
		if err != nil {
			return err
		}
		var out bytes.Buffer
		if err := t.Execute(&out, d); err != nil {
			return err
		}
		content := out.Bytes()
		if strings.HasSuffix(rel, ".go") {
			content, err = format.Source(content)
			if err != nil {
				return fmt.Errorf("format %s: %w", rel, err)
			}
		}
		files[rel] = content
		return nil
	})
	return files, err
}

// writeFiles confines access to root, rejects symlink parents, and rolls back only
// files/directories created by this invocation. Existing files are never removed.
func writeFiles(ctx context.Context, root *os.Root, files map[string][]byte) (err error) {
	names := make([]string, 0, len(files))
	for name := range files {
		if !fs.ValidPath(name) {
			return fmt.Errorf("unsafe template path %q", name)
		}
		names = append(names, name)
	}
	slices.Sort(names)
	var createdFiles, createdDirs []string
	defer func() {
		if err == nil {
			return
		}
		for _, name := range slices.Backward(createdFiles) {
			err = errors.Join(err, root.Remove(name))
		}
		for _, name := range slices.Backward(createdDirs) {
			err = errors.Join(err, root.Remove(name))
		}
	}()
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		parts := strings.Split(path.Dir(name), "/")
		for i := range parts {
			parent := strings.Join(parts[:i+1], "/")
			if parent == "." {
				continue
			}
			info, statErr := root.Lstat(parent)
			switch {
			case errors.Is(statErr, fs.ErrNotExist):
				if err := root.Mkdir(parent, 0o700); err != nil {
					return err
				}
				createdDirs = append(createdDirs, parent)
			case statErr != nil:
				return statErr
			case !info.IsDir() || info.Mode()&os.ModeSymlink != 0:
				return fmt.Errorf("unsafe output directory %s", parent)
			}
		}
		f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("create %s (existing files are never overwritten): %w", name, err)
		}
		createdFiles = append(createdFiles, name)
		_, writeErr := f.Write(files[name])
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
	}
	return ctx.Err()
}
