//go:build integration

package scaffold_test

import (
	"context"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGeneratedSmoke(t *testing.T) {
	if os.Getenv("NOSTOSX_GENERATED_SMOKE") != "1" {
		t.Skip("run make smoke to exercise generated application processes")
	}
	cli, source := buildCLI(t)
	forGeneratedVariants(t, cli, source, func(t *testing.T, project, layout string) {
		runProject(t, project, "make", "api-check", "smoke", "NOSTOSX="+cli)
		extendProject(t, cli, source, project, true)
		fixture, err := os.ReadFile("testdata/api_integration_test.go.txt")
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(project, "services/order/api_integration_test.go"), string(fixture))
		if layout == "monolith" {
			assembleServices(t, project)
		}
		runProject(t, project, goExecutable(), "test", "-mod=readonly", "-tags", "integration", "-race", "-shuffle=on", "-count=1", "./...")
		runProject(t, project, "make", "smoke")
	})
}

func TestTemplateGates(t *testing.T) {
	gate := os.Getenv("NOSTOSX_TEMPLATE_GATE")
	if gate != "lint" && gate != "vulncheck" {
		t.Skip("run make templates-lint or templates-vulncheck")
	}
	cli, source := buildCLI(t)
	forGeneratedVariants(t, cli, source, func(t *testing.T, project, _ string) {
		runProject(t, project, "make", gate)
		extendProject(t, cli, source, project, false)
		runProject(t, project, "make", gate)
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

func forGeneratedVariants(t *testing.T, cli, source string, check func(*testing.T, string, string)) {
	t.Helper()
	for _, layout := range []string{"monolith", "microservices"} {
		for _, agent := range []bool{false, true} {
			name := layout + "/base"
			if agent {
				name = layout + "/agentx"
			}
			t.Run(name, func(t *testing.T) {
				project := filepath.Join(t.TempDir(), "workspace")
				args := []string{"new", "workspace", "--module", "example.com/acme/workspace", "--layout", layout, "--output", project}
				if agent {
					args = append(args, "--with", "agentx")
				}
				runProject(t, source, cli, args...)
				runProject(t, project, goExecutable(), "mod", "download")
				runProject(t, project, goExecutable(), "mod", "verify")
				runProject(t, source, cli, "doctor", "--dir", project)
				check(t, project, layout)
			})
		}
	}
}

func extendProject(t *testing.T, cli, source, project string, shipping bool) {
	t.Helper()
	names := []string{"order"}
	if shipping {
		names = append(names, "shipping")
	}
	for _, name := range names {
		for _, kind := range []string{"context", "service"} {
			runProject(t, source, cli, "add", kind, name, "--dir", project)
		}
	}
	runProject(t, source, cli, "add", "api", "create-order", "--service", "order", "--method", "POST", "--path", "/v1/orders", "--dir", project)
	runProject(t, source, cli, "add", "api", "get-order", "--service", "order", "--method", "GET", "--path", "/v1/orders/{order_id}", "--dir", project)
	runProject(t, source, cli, "add", "api", "ping", "--service", "order", "--dir", project)
	proto := filepath.Join(project, "api/proto/order/v1/order.proto")
	code := read(t, proto)
	code = strings.Replace(code, "service OrderService {", "service OrderService {\n  rpc Inspect(InspectRequest) returns (InspectResponse);", 1)
	code += "\nmessage InspectRequest {}\nmessage InspectResponse {}\n"
	write(t, proto, code)
	runProject(t, source, cli, "api", "scaffold", "--service", "order", "--dir", project)
	runProject(t, source, cli, "api", "scaffold", "--service", "order", "--dir", project)
	runProject(t, source, cli, "doctor", "--json", "--dir", project)
	generateAPI(t, project, cli)
	runProject(t, project, goExecutable(), "get", "github.com/grpc-ecosystem/grpc-gateway/v2@v2.29.0")
	runProject(t, project, goExecutable(), "mod", "tidy")
}

func assembleServices(t *testing.T, project string) {
	t.Helper()
	name := filepath.Join(project, "cmd/server/main.go")
	code := read(t, name)
	if !strings.Contains(code, "os.Stderr, nil)") {
		t.Fatal("monolith assembly call not found")
	}
	code = strings.Replace(code, "import (", `import (
	"github.com/nostosx/servex/transport/httpx"
	"google.golang.org/grpc"
	"example.com/acme/workspace/services/order"
	"example.com/acme/workspace/services/shipping"`, 1)
	code = strings.Replace(code, "os.Stderr, nil)", `os.Stderr, func(ctx context.Context, http *httpx.Server, rpc grpc.ServiceRegistrar) error {
		if err := order.Register(ctx, http, rpc); err != nil { return err }
		return shipping.Register(ctx, http, rpc)
	})`, 1)
	formatted, err := format.Source([]byte(code))
	if err != nil {
		t.Fatal(err)
	}
	write(t, name, string(formatted))
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
