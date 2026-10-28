//go:build integration

package scaffold_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

// Exercise real Go tooling in standalone generated modules, never sibling replaces.
func TestGeneratedProjects(t *testing.T) {
	binary, source := buildCLI(t)
	forGeneratedVariants(t, binary, source, func(t *testing.T, project string) {
		checkTaskCommands(t, project)
		checkAPI(t, project, binary)
		checkInitialProject(t, project, goExecutable())
		extendProject(t, binary, source, project)
		before := snapshot(t, project)
		if !strings.Contains(before["infrastructure/server/server.go"], "*httpx.Server") && strings.Contains(before["go.mod"], "grpc-gateway") {
			t.Fatal("gRPC-only API unexpectedly requires the gateway runtime")
		}
		runProject(t, project, goExecutable(), "mod", "verify")
		runProject(t, project, "task", "check", "INTEGRATION=true", "GO="+goExecutable())
		for _, name := range []string{"go.mod", "go.sum"} {
			if read(t, filepath.Join(project, name)) != before[name] {
				t.Fatalf("checks modified %s", name)
			}
		}
	})
}

func TestInitialProjectWithColdModuleCache(t *testing.T) {
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOFLAGS", strings.TrimSpace(os.Getenv("GOFLAGS")+" -modcacherw"))
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	goTool := os.Getenv("GO")
	if goTool == "" {
		goTool = "go"
	}
	checkInitialProject(t, o.Output, goTool)
}

func TestAddedNamesAvoidImportCollisions(t *testing.T) {
	binary, _ := buildCLI(t)
	o := options(t)
	o.Transport = "http,grpc"
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"context", "cqrs", "query", "testing", "errors", "flag", "slog", "os", "strings", "time", "app", "health", "http", "httpx", "observability", "domain", "service", "init"} {
		for _, kind := range []string{"context", "service"} {
			if err := scaffold.Add(t.Context(), o.Output, kind, name); err != nil {
				t.Fatalf("add %s %s: %v", kind, name, err)
			}
		}
	}
	generateAPI(t, o.Output, binary)
	goTool := os.Getenv("GO")
	if goTool == "" {
		goTool = "go"
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	tidy := exec.CommandContext(ctx, goTool, "mod", "tidy")
	tidy.Dir = o.Output
	tidy.Env = append(os.Environ(), "GOWORK=off")
	if output, err := tidy.CombinedOutput(); err != nil {
		t.Fatalf("tidy added services: %v\n%s", err, output)
	}
	cmd := exec.CommandContext(ctx, goTool, "test", "-mod=readonly", "./application/...", "./services/...")
	cmd.Dir = o.Output
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("check added contexts and services: %v\n%s", err, output)
	}
}

func generateAPI(t *testing.T, dir, binary string) {
	t.Helper()
	runProject(t, dir, "task", "api", "ACTION=generate", "NOSTOSX="+binary)
	runProject(t, dir, "task", "api", "NOSTOSX="+binary)
}

func checkTaskCommands(t *testing.T, dir string) {
	t.Helper()
	var listing struct {
		Tasks []struct {
			Name string `json:"name"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(runProject(t, dir, "task", "--list-all", "--json"), &listing); err != nil {
		t.Fatal(err)
	}
	want := []string{"build", "check", "fix", "fmt", "lint", "test", "tidy", "vulncheck"}
	if _, err := os.Stat(filepath.Join(dir, "buf.yaml")); err == nil {
		want = append(want, "api")
	}
	slices.Sort(want)
	names := make([]string, len(listing.Tasks))
	for i, task := range listing.Tasks {
		names[i] = task.Name
	}
	slices.Sort(names)
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("public Task commands = %v, want %v", names, want)
	}
}

// Verify the initial empty contract state without requiring placeholder APIs.
func checkAPI(t *testing.T, dir, binary string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "api", "check", "--dir", dir)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verify embedded contracts: %v\n%s", err, output)
	}
}

func checkInitialProject(t *testing.T, dir, goTool string) {
	t.Helper()
	runProject(t, dir, "task", "check", "INTEGRATION=true", "GO="+goTool)
}
