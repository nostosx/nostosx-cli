//go:build integration

package scaffold_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

// Exercise real Go tooling in standalone generated modules, never sibling replaces.
func TestGeneratedProjects(t *testing.T) {
	binary, _ := buildCLI(t)
	goTool := os.Getenv("GO")
	if goTool == "" {
		goTool = "go"
	}
	for _, layout := range []string{"monolith", "microservices"} {
		for _, agent := range []bool{false, true} {
			t.Run(layout+map[bool]string{true: "/agentx", false: "/base"}[agent], func(t *testing.T) {
				o := options(t)
				o.Layout, o.Agent = layout, agent
				o.Module = "x.dev/s"
				if agent {
					o.Module = "example.com/organization/platform/backend/with/a/long/module/path"
				}
				if _, err := scaffold.New(t.Context(), o); err != nil {
					t.Fatal(err)
				}
				checkAPI(t, o.Output, binary)
				checkInitialProject(t, o.Output, goTool)
				for _, kind := range []string{"context", "service"} {
					if err := scaffold.Add(t.Context(), o.Output, kind, "order"); err != nil {
						t.Fatal(err)
					}
				}
				if err := scaffold.AddAPI(t.Context(), o.Output, scaffold.APIOptions{Name: "ping", Service: "order"}); err != nil {
					t.Fatal(err)
				}
				generateAPI(t, o.Output, binary)
				runProject(t, o.Output, goTool, "mod", "tidy")
				runProject(t, o.Output, goTool, "build", "-mod=readonly", "./...")
				if strings.Contains(read(t, filepath.Join(o.Output, "go.mod")), "grpc-ecosystem/grpc-gateway") {
					t.Fatal("gRPC-only API unexpectedly requires gateway runtime")
				}
				for _, api := range []scaffold.APIOptions{
					{Name: "create-order", Service: "order", Method: "POST", Path: "/v1/orders"},
					{Name: "get-order", Service: "order", Method: "GET", Path: "/v1/orders/{order_id}"},
				} {
					if err := scaffold.AddAPI(t.Context(), o.Output, api); err != nil {
						t.Fatal(err)
					}
				}
				if layout == "monolith" && !agent {
					obsolete := filepath.Join(o.Output, "api/gen/go/obsolete.pb.go")
					if err := os.MkdirAll(filepath.Dir(obsolete), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(obsolete, []byte("package obsolete\n"), 0o600); err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
					cmd := exec.CommandContext(ctx, "make", "api-check", "NOSTOSX="+binary)
					cmd.Dir = o.Output
					output, err := cmd.CombinedOutput()
					cancel()
					if err == nil || !strings.Contains(string(output), "generated contracts differ") {
						t.Fatalf("stale generated file accepted: %v\n%s", err, output)
					}
				}
				generateAPI(t, o.Output, binary)
				gateway := exec.Command(goTool, "get", "github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0")
				gateway.Dir = o.Output
				gateway.Env = append(os.Environ(), "GOWORK=off")
				if output, err := gateway.CombinedOutput(); err != nil {
					t.Fatalf("install gateway runtime: %v\n%s", err, output)
				}
				tidy := exec.Command(goTool, "mod", "tidy")
				tidy.Dir = o.Output
				tidy.Env = append(os.Environ(), "GOWORK=off")
				if output, err := tidy.CombinedOutput(); err != nil {
					t.Fatalf("tidy added contract: %v\n%s", err, output)
				}
				before := snapshot(t, o.Output)
				for _, args := range [][]string{
					{"mod", "download"}, {"mod", "verify"}, {"mod", "tidy", "-diff"},
					{"fix", "-diff", "./..."}, {"fix", "-tags", "integration", "-diff", "./..."},
					{"vet", "-tags", "integration", "./..."},
					{"test", "-mod=readonly", "-tags", "integration", "-race", "-shuffle=on", "-count=1", "./..."},
					{"build", "-mod=readonly", "./..."},
				} {
					t.Run(strings.Join(args[:2], "-"), func(t *testing.T) {
						ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
						defer cancel()
						cmd := exec.CommandContext(ctx, goTool, args...)
						cmd.Dir = o.Output
						cmd.Env = append(os.Environ(), "GOWORK=off")
						output, err := cmd.CombinedOutput()
						mustBeEmpty := args[0] == "fix" || (args[0] == "mod" && args[1] == "tidy")
						if err != nil || (mustBeEmpty && len(output) != 0) {
							t.Fatalf("go %v: %v\n%s", args, err, output)
						}
					})
				}
				for _, name := range []string{"go.mod", "go.sum"} {
					if got := read(t, filepath.Join(o.Output, name)); got != before[name] {
						t.Fatalf("checks modified generated %s", name)
					}
				}
			})
		}
	}
}

func TestAddedNamesAvoidImportCollisions(t *testing.T) {
	binary, _ := buildCLI(t)
	o := options(t)
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
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "make", "generate", "api-check", "NOSTOSX="+binary)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate contracts: %v\n%s", err, output)
	}
}

// Verify the initial empty contract state without requiring placeholder APIs.
func checkAPI(t *testing.T, dir, binary string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "make", "api-check", "NOSTOSX="+binary)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("verify embedded contracts: %v\n%s", err, output)
	}
}

func checkInitialProject(t *testing.T, dir, goTool string) {
	t.Helper()
	for _, args := range [][]string{{"mod", "tidy", "-diff"}, {"test", "-mod=readonly", "-tags", "integration", "-race", "-shuffle=on", "-count=1", "./..."}} {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		cmd := exec.CommandContext(ctx, goTool, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off")
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil || (args[0] == "mod" && len(output) != 0) {
			t.Fatalf("initial go %v: %v\n%s", args, err, output)
		}
	}
}
