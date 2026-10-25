// Package doctor performs read-only environment and generated-project checks.
// Tool version commands execute installed Go and Buf binaries; they must be trusted.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"go/version"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/nostosx/nostosx-cli/internal/contract"
)

// Check is one diagnostic. Status is pass, fail or skip.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Report has a stable JSON shape suitable for CI and local tooling.
type Report struct {
	Version string  `json:"version"`
	Checks  []Check `json:"checks"`
}

// Healthy reports whether all required checks passed.
func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if check.Status == "fail" {
			return false
		}
	}
	return true
}

type runner func(context.Context, string, ...string) (string, error)

// Run inspects the project and installed tool versions without installing tools,
// fetching dependencies, running project hooks or changing project files.
func Run(ctx context.Context, dir, cliVersion string) Report {
	return inspect(ctx, dir, cliVersion, toolVersion)
}

func inspect(ctx context.Context, dir, cliVersion string, run runner) Report {
	dir = filepath.Clean(dir)
	report := Report{Version: cliVersion, Checks: make([]Check, 0)}
	add := func(name, detail, fix string, err error) {
		status := "pass"
		if err != nil {
			status, detail = "fail", err.Error()
		} else {
			fix = ""
		}
		report.Checks = append(report.Checks, Check{Name: name, Status: status, Detail: detail, Fix: fix})
	}
	output, err := run(ctx, "go", "version")
	fields := strings.Fields(output)
	installedGo := ""
	if err == nil && len(fields) >= 3 && version.IsValid(fields[2]) {
		installedGo = fields[2]
	}
	if err == nil && (len(fields) < 3 || !version.IsValid(fields[2]) || version.Compare(fields[2], "go1.27.3") < 0) {
		err = errors.New("require Go 1.27.3 or newer")
	}
	add("go", strings.TrimSpace(output), "Install Go 1.27.3 or newer and check PATH/GOROOT.", err)
	info, err := os.Lstat(dir)
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		err = errors.New("project root must be a real directory")
	}
	if err != nil {
		add("project", "", "Pass a generated project with --dir.", err)
		return report
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		add("project", "", "Pass an accessible project with --dir.", err)
		return report
	}
	defer root.Close()
	b, err := read(root, "go.mod")
	if err == nil {
		var file *modfile.File
		file, err = modfile.Parse("go.mod", b, nil)
		if err == nil && (file.Module == nil || file.Go == nil || !version.IsValid("go"+file.Go.Version) || version.Compare("go"+file.Go.Version, "go1.27.3") < 0) {
			err = errors.New("go.mod requires a module and go 1.27.3 or newer")
		}
		if err == nil {
			err = module.CheckPath(file.Module.Mod.Path)
		}
		if err == nil && installedGo != "" && version.Compare(installedGo, "go"+file.Go.Version) < 0 {
			err = fmt.Errorf("project requires Go %s; installed %s", file.Go.Version, installedGo)
		}
	}
	add("module", "go.mod is valid", "Run doctor from a generated project; check its module and Go directives.", err)
	b, err = read(root, "buf.yaml")
	if err == nil {
		var config struct {
			Version string `yaml:"version"`
			Modules []struct {
				Path string `yaml:"path"`
			} `yaml:"modules"`
		}
		err = yaml.Unmarshal(b, &config)
		if err == nil && (config.Version != "v2" || len(config.Modules) != 1 || config.Modules[0].Path != "api/proto") {
			err = errors.New("buf.yaml must use v2 with the api/proto module")
		}
	}
	add("buf-config", "Buf configuration matches the project layout", "Restore buf.yaml with version v2 and module path api/proto.", err)
	count, err := protoCount(ctx, root)
	add("proto", fmt.Sprintf("%d local Proto files", count), "Check api/proto; symlinks and oversized trees are unsupported.", err)
	if err == nil && count == 0 {
		report.Checks = append(report.Checks, Check{Name: "buf", Status: "skip", Detail: "No Proto contracts; Buf is not required yet"})
	} else if err == nil {
		output, err = run(ctx, "buf", "--version")
		if err == nil && strings.TrimSpace(output) != contract.RequiredBufVersion {
			err = fmt.Errorf("require Buf %s", contract.RequiredBufVersion)
		}
		add("buf", strings.TrimSpace(output), "Install Buf "+contract.RequiredBufVersion+" and check PATH.", err)
	}
	return report
}

func read(root *os.Root, name string) ([]byte, error) {
	for parent := name; parent != "."; parent = path.Dir(parent) {
		info, err := root.Lstat(parent)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink is unsupported: %s", parent)
		}
		if parent == name && (!info.Mode().IsRegular() || info.Size() > 1<<20) {
			return nil, fmt.Errorf("%s must be a regular file of at most 1 MiB", name)
		}
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("%s must be a regular file of at most 1 MiB", name)
	}
	b, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if len(b) > 1<<20 {
		return nil, fmt.Errorf("%s exceeds 1 MiB", name)
	}
	return b, err
}

func protoCount(ctx context.Context, root *os.Root) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	for _, name := range []string{"api", "api/proto"} {
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) && name == "api/proto" {
			return 0, nil // Clean skeletons have no contract directory yet.
		}
		if err != nil {
			return 0, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("%s must be a real directory", name)
		}
	}
	count, entries, size := 0, 0, 0
	err := fs.WalkDir(root.FS(), "api/proto", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > 512 || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe or oversized Proto tree: %s", name)
		}
		if !entry.IsDir() && strings.HasSuffix(name, ".proto") {
			b, err := read(root, name)
			if err != nil {
				return err
			}
			size += len(b)
			if size > 16<<20 {
				return errors.New("proto tree exceeds 16 MiB")
			}
			count++
		}
		return nil
	})
	return count, err
}

type limitedOutput struct{ text strings.Builder }

func (o *limitedOutput) Write(p []byte) (int, error) {
	if o.text.Len()+len(p) > 4096 {
		return 0, errors.New("tool version output exceeds 4 KiB")
	}
	return o.text.Write(p)
}

func toolVersion(ctx context.Context, tool string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	// Prevent Go version probing from downloading a toolchain or loading GOENV.
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOENV=off")
	cmd.WaitDelay = time.Second
	var output limitedOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cannot query %s version: %w", tool, err)
	}
	return output.text.String(), nil
}
