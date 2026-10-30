//go:build integration

package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify generated routing, contract conversion, one shared chain, metadata and
// error propagation against actual HTTP and gRPC requests in a standalone module.
func TestGeneratedCQRSRoundTrip(t *testing.T) {
	cli, source := buildCLI(t)
	project := filepath.Join(t.TempDir(), "project")
	runProject(t, source, cli, "new", "project", "--module", "example.com/cqrs", "--transport", "http,grpc", "--output", project)
	runProject(t, source, cli, "add", "service", "studio", "--dir", project)
	runProject(t, source, cli, "add", "context", "creation", "--dir", project)
	runProject(t, source, cli, "add", "api", "--context", "creation", "get-order", "--service", "studio", "--method", "GET", "--path", "/orders/{order_id}", "--dir", project)
	runProject(t, source, cli, "add", "api", "--context", "creation", "get-order", "--service", "studio", "--transport", "grpc", "--kind", "query", "--dir", project)
	runProject(t, source, cli, "add", "api", "--context", "creation", "create-order", "--service", "studio", "--method", "POST", "--path", "/orders", "--dir", project)
	generateAPI(t, project, cli)
	// Implement only the business body. Adapters and assembly stay generated.
	name := filepath.Join(project, "application/creation/query/get_order.go")
	code := strings.Replace(read(t, name), "_ GetOrderQuery", "input GetOrderQuery", 1)
	code = strings.Replace(code, `return GetOrderResult{}, errorx.New(errorx.Unimplemented, "NOT_IMPLEMENTED", "operation is not implemented")`, `if input.OrderId != "42" { return GetOrderResult{}, errorx.New(errorx.InvalidArgument,"INVALID_ID","id must be 42") }; return GetOrderResult{},nil`, 1)
	write(t, name, code)
	app := filepath.Join(project, "services/studio/internal/application/query.go")
	code = strings.Replace(read(t, app), `deps.GetOrder)`, `deps.GetOrder, observeGetOrder)`, 1)
	write(t, app, code)
	fixture, err := os.ReadFile("testdata/cqrs/observe.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(project, "services/studio/internal/application/observe.go"), string(fixture))
	fixture, err = os.ReadFile("testdata/cqrs/roundtrip_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(project, "services/studio/cqrs_test.go"), string(fixture))
	runProject(t, project, goExecutable(), "fmt", "./...")
	runProject(t, project, goExecutable(), "mod", "tidy")
	runProject(t, project, goExecutable(), "test", "-race", "-count=1", "./services/studio")
}
