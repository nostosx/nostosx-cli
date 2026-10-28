//go:build integration

package scaffold_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTemplateGates(t *testing.T) {
	gate := os.Getenv("NOSTOSX_TEMPLATE_GATE")
	if gate != "lint" && gate != "vulncheck" {
		t.Skip("run task lint SCOPE=templates or task vulncheck SCOPE=templates")
	}
	cli, source := buildCLI(t)
	forGeneratedVariants(t, cli, source, func(t *testing.T, project string) {
		runProject(t, project, "task", gate)
		extendProject(t, cli, source, project)
		runProject(t, project, "task", gate)
	})
}

func buildCLI(t *testing.T) (string, string) {
	t.Helper()
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	name := "nostosx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), "tool with spaces", name)
	if err := os.MkdirAll(filepath.Dir(binary), 0o700); err != nil {
		t.Fatal(err)
	}
	runProject(t, source, goExecutable(), "build", "-mod=readonly", "-o", binary, "./cmd/nostosx")
	return binary, source
}

func forGeneratedVariants(t *testing.T, cli, source string, check func(*testing.T, string)) {
	t.Helper()
	for _, layout := range []string{"monolith", "microservices"} {
		for _, agent := range []bool{false, true} {
			for _, transport := range []string{"http", "grpc", "http,grpc"} {
				name := layout + "/base"
				if agent {
					name = layout + "/agentx"
				}
				t.Run(name+"/"+transport, func(t *testing.T) {
					project := filepath.Join(t.TempDir(), "workspace")
					module := "x.dev/s"
					if agent {
						module = "example.com/organization/platform/backend/with/a/long/module/path"
					}
					args := []string{"new", "workspace", "--module", module, "--layout", layout, "--transport", transport, "--output", project}
					if agent {
						args = append(args, "--with", "agentx")
					}
					runProject(t, source, cli, args...)
					runProject(t, project, goExecutable(), "mod", "download")
					runProject(t, project, goExecutable(), "mod", "verify")
					runProject(t, source, cli, "doctor", "--dir", project)
					check(t, project)
				})
			}
		}
	}
}

func extendProject(t *testing.T, cli, source, project string) {
	t.Helper()
	for _, kind := range []string{"context", "service"} {
		runProject(t, source, cli, "add", kind, "order", "--dir", project)
	}
	server := read(t, filepath.Join(project, "infrastructure/server/server.go"))
	hasHTTP := strings.Contains(server, "*httpx.Server")
	_, protoErr := os.Stat(filepath.Join(project, "buf.yaml"))
	hasGRPC := protoErr == nil
	if hasHTTP {
		runProject(t, source, cli, "add", "api", "create-order", "--service", "order", "--method", "POST", "--path", "/v1/orders", "--dir", project)
		runProject(t, source, cli, "add", "api", "get-order", "--service", "order", "--method", "GET", "--path", "/v1/orders/{order_id}", "--dir", project)
	}
	if hasGRPC {
		runProject(t, source, cli, "add", "api", "ping", "--transport", "grpc", "--service", "order", "--dir", project)
		if hasHTTP {
			runProject(t, source, cli, "add", "api", "gateway-order", "--transport", "gateway", "--service", "order", "--method", "GET", "--path", "/v1/gateway/orders/{order_id}", "--dir", project)
		}
		proto := filepath.Join(project, "api/proto/order/v1/order.proto")
		code := read(t, proto)
		code = strings.Replace(code, "service OrderService {", "service OrderService {\n  rpc Inspect(InspectRequest) returns (InspectResponse);", 1)
		code += "\nmessage InspectRequest {}\nmessage InspectResponse {}\n"
		write(t, proto, code)
		runProject(t, source, cli, "api", "scaffold", "--service", "order", "--dir", project)
		runProject(t, source, cli, "api", "scaffold", "--service", "order", "--dir", project)
		generateAPI(t, project, cli)
		if hasHTTP {
			runProject(t, project, goExecutable(), "get", "github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0")
		}
	}
	runProject(t, source, cli, "doctor", "--json", "--dir", project)
	runProject(t, project, goExecutable(), "mod", "tidy")
}

func goExecutable() string {
	if tool := os.Getenv("GO"); tool != "" {
		return tool
	}
	return "go"
}

func write(t *testing.T, name, data string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runProject(t *testing.T, dir, tool string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s in %s: %v\n%s", tool, strings.Join(args, " "), dir, err, output)
	}
	t.Logf("%s %s\n%s", tool, strings.Join(args, " "), output)
	return output
}
