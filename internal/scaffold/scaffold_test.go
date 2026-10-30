package scaffold_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func options(t *testing.T) scaffold.Options {
	t.Helper()
	return scaffold.Options{Name: "shop", Module: "example.com/acme/shop", Layout: "monolith", Output: filepath.Join(t.TempDir(), "shop")}
}

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = read(t, name)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestNewLayouts(t *testing.T) {
	t.Parallel()
	for _, layout := range []string{"monolith", "microservices"} {
		for _, agent := range []bool{false, true} {
			for _, transport := range []string{"http", "grpc", "http,grpc"} {
				t.Run(layout+map[bool]string{true: "/agentx/", false: "/business/"}[agent]+transport, func(t *testing.T) {
					t.Parallel()
					o := options(t)
					o.Layout, o.Agent, o.Transport = layout, agent, transport
					if _, err := scaffold.New(t.Context(), o); err != nil {
						t.Fatal(err)
					}
					files := snapshot(t, o.Output)
					for name := range files {
						if strings.HasPrefix(name, "domain/") || strings.HasPrefix(name, "application/") || strings.HasPrefix(name, "services/") || strings.HasPrefix(name, "api/proto/") || strings.HasPrefix(name, "api/gen/") {
							t.Errorf("new project includes an unsolicited context or contract: %s", name)
						}
						for _, forbidden := range []string{"catalog", "inventory", "archtest", ".nostosx.json", "get_status", "database/", "cmd/migrate/", "scripts/", "apigen", "Makefile", "bootstrap"} {
							if strings.Contains(name, forbidden) {
								t.Errorf("unexpected generated path %s", name)
							}
						}
					}
					for _, name := range []string{"infrastructure/config/config.go", "infrastructure/server/server.go", "infrastructure/logging/logging.go", "Taskfile.yml"} {
						if _, ok := files[name]; !ok {
							t.Errorf("missing generated file %s", name)
						}
					}
					for _, name := range []string{"AGENTS.md", "CONTRIBUTING.md", "README.md", "README.zh-CN.md", "CHANGELOG.md", "SECURITY.md", "LICENSE"} {
						if _, ok := files[name]; ok {
							t.Errorf("unsolicited generated document %s", name)
						}
					}
					for name, content := range files {
						if (strings.HasSuffix(name, ".go") && strings.Contains(content, "github.com/nostosx/nostosx-cli")) || strings.Contains(content, "[[.Module]]") {
							t.Fatalf("unrendered module in %s", name)
						}
						if strings.HasSuffix(name, ".go") {
							if strings.Contains(content, "RunCLI") {
								t.Fatalf("generated command still delegates process startup in %s", name)
							}
							if _, err := parser.ParseFile(token.NewFileSet(), name, content, parser.AllErrors); err != nil {
								t.Fatal(err)
							}
							if !agent && strings.Contains(content, "github.com/nostosx/agentx") {
								t.Fatalf("agentx leaked into %s", name)
							}
						}
					}
					_, combined := files["cmd/server/main.go"]
					_, composition := files["app.go"]
					if composition != combined {
						t.Fatal("monolith entry and app.go must be generated together")
					}
					if combined {
						entry := files["cmd/server/main.go"]
						if !strings.Contains(entry, "application.Run(ctx, cfg)") || strings.Contains(entry, "/infrastructure/server") || strings.Contains(entry, "app.New(") {
							t.Fatal("monolith entry must delegate assembly to app.go")
						}
					}
					if _, exists := files["config/config.toml"]; exists != combined {
						t.Fatal("only a monolith has a root process configuration")
					}
					_, agentRunner := files["infrastructure/agent/runner.go"]
					if combined != (layout == "monolith") || agentRunner != agent {
						t.Fatal("incorrect topology or optional feature")
					}
					if strings.Contains(files["go.mod"], "agentx") != agent {
						t.Fatal("incorrect agent dependency")
					}
					o.Output = filepath.Join(t.TempDir(), "second")
					if _, err := scaffold.New(t.Context(), o); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(files, snapshot(t, o.Output)) {
						t.Fatal("generation is not reproducible")
					}
				})
			}
		}
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		edit func(*scaffold.Options)
	}{
		{"traversal", func(o *scaffold.Options) { o.Name = "../escape" }},
		{"windows device", func(o *scaffold.Options) { o.Name = "con" }},
		{"keyword", func(o *scaffold.Options) { o.Name = "func" }},
		{"layout", func(o *scaffold.Options) { o.Layout = "cluster" }},
		{"module newline", func(o *scaffold.Options) { o.Module = "example.com/shop\nreplace a => b" }},
		{"module traversal", func(o *scaffold.Options) { o.Module = "example.com/../shop" }},
		{"module template", func(o *scaffold.Options) { o.Module = "example.com/[[.Name]]" }},
		{"module uppercase", func(o *scaffold.Options) { o.Module = "example.com/Shop" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := options(t)
			tc.edit(&o)
			if _, err := scaffold.New(t.Context(), o); err == nil {
				t.Fatal("invalid input accepted")
			}
			if _, err := os.Lstat(o.Output); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("invalid input created output")
			}
		})
	}
}

func TestNewPreservesExistingDirectory(t *testing.T) {
	t.Parallel()
	o := options(t)
	if err := os.Mkdir(o.Output, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Output, "sentinel"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffold.New(t.Context(), o); err == nil {
		t.Fatal("existing destination accepted")
	}
	if got := snapshot(t, o.Output); !reflect.DeepEqual(got, map[string]string{"sentinel": "keep"}) {
		t.Fatalf("destination changed: %v", got)
	}
}

func TestNewCanceledAndConcurrent(t *testing.T) {
	t.Parallel()
	o := options(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := scaffold.New(ctx, o); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if _, err := os.Stat(o.Output); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("canceled generation left output")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := scaffold.New(t.Context(), o); results <- err })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 || read(t, filepath.Join(o.Output, "go.mod")) == "" {
		t.Fatalf("successful concurrent generations=%d", success)
	}
}

func TestAddPreservesFilesAndRollsBack(t *testing.T) {
	t.Parallel()
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, o.Output)
	if err := scaffold.Add(t.Context(), o.Output, "service", "studio"); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Add(t.Context(), o.Output, "service", "order"); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, o.Output)
	for _, name := range []string{
		"application/order/query/doc.go",
		"domain/order/doc.go",
		"services/order/internal/port/http/server.go",
		"services/order/config/config.toml",
	} {
		if _, ok := after[name]; !ok {
			t.Errorf("missing added file %s", name)
		}
	}
	for name, content := range before {
		if after[name] != content {
			t.Fatalf("existing file changed: %s", name)
		}
	}
	if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if !reflect.DeepEqual(after, snapshot(t, o.Output)) {
		t.Fatal("failed addition changed files")
	}
	// Force a conflict after earlier paths would have been written.
	if err := os.MkdirAll(filepath.Join(o.Output, "domain", "payment"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Output, "domain", "payment", "doc.go"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	before = snapshot(t, o.Output)
	if err := scaffold.Add(t.Context(), o.Output, "context", "payment"); err == nil {
		t.Fatal("conflicting addition accepted")
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("failed addition was not rolled back")
	}
}

func TestSymlinksCannotRedirectWrites(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires developer mode or elevated privileges")
	}
	o := options(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, o.Output); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffold.New(t.Context(), o); err == nil {
		t.Fatal("symlink destination accepted")
	}
	if err := os.Remove(o.Output); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(o.Output, "application"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(o.Output, "application", "order")); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err == nil {
		t.Fatal("symlink parent accepted")
	}
	if len(snapshot(t, outside)) != 0 {
		t.Fatal("generator wrote outside project")
	}
}

func TestAddUsesCurrentModulePath(t *testing.T) {
	t.Parallel()
	// Add reads the current go.mod and server construction shape, never project metadata.
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	dir := o.Output
	const module = "example.com/renamed/shop/v2"
	goMod := "module " + module + "\n\ngo 1.27.3\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"context", "service"} {
		if err := scaffold.Add(t.Context(), dir, kind, "order"); err != nil {
			t.Fatal(err)
		}
	}
	files := snapshot(t, dir)
	if files["go.mod"] != goMod {
		t.Fatal("addition rewrote go.mod")
	}
	for _, name := range []string{
		"services/order/app.go",
	} {
		if !strings.Contains(files[name], `"`+module+`/`) {
			t.Fatalf("%s does not use the current module path", name)
		}
	}
	if _, ok := files[".nostosx.json"]; ok {
		t.Fatal("addition created redundant project metadata")
	}
}

func TestAddRejectsInvalidGoModule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		content string
		missing bool
	}{
		{name: "missing file", missing: true},
		{name: "missing module", content: "go 1.27.3\n"},
		{name: "invalid path", content: "module example.com/../shop\n"},
		{name: "uppercase path", content: "module example.com/Shop\n"},
		{name: "malformed directives", content: "module example.com/acme/shop\nrequire (\n"},
		{name: "duplicate module", content: "module example.com/acme/shop\nmodule example.com/other/shop\n"},
		{name: "oversized", content: "module example.com/acme/shop\n" + strings.Repeat(" ", 1<<20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := options(t)
			if _, err := scaffold.New(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			goMod := filepath.Join(o.Output, "go.mod")
			if tc.missing {
				if err := os.Remove(goMod); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(goMod, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, o.Output)
			if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err == nil {
				t.Fatal("invalid go.mod accepted")
			}
			if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
				t.Fatal("rejected addition changed project files or left a lock")
			}
		})
	}
}

func TestAddHonorsCancellationAndExistingLock(t *testing.T) {
	t.Parallel()
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, o.Output)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := scaffold.Add(ctx, o.Output, "context", "order"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("canceled addition changed project files or left a lock")
	}
	if err := os.WriteFile(filepath.Join(o.Output, ".nostosx.lock"), []byte("another invocation"), 0o600); err != nil {
		t.Fatal(err)
	}
	before = snapshot(t, o.Output)
	if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err == nil {
		t.Fatal("existing lock ignored")
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("rejected addition changed files or removed another invocation's lock")
	}
}
