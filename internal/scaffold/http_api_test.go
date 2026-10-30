package scaffold_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func httpProject(t *testing.T) string {
	t.Helper()
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"context", "service"} {
		if err := scaffold.Add(t.Context(), o.Output, kind, "order"); err != nil {
			t.Fatal(err)
		}
	}
	return o.Output
}

func TestNativeHTTPNeedsNoContractTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := httpProject(t)
	before := snapshot(t, dir)
	o := scaffold.APIOptions{Context: "order", Name: "get-order", Service: "order", Method: "GET", Path: "/v1/orders/{order_id}"}
	if err := scaffold.AddAPI(t.Context(), dir, o); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, dir)
	if len(after) != len(before)+2 {
		t.Fatalf("expected adapter and application use case, got %d files", len(after)-len(before))
	}
	for name, content := range before {
		if after[name] != content && name != "services/order/app.go" && name != "services/order/internal/port/http/server.go" && name != "services/order/internal/application/query.go" && name != "services/order/internal/port/http/mapper.go" {
			t.Fatalf("rewrote %s", name)
		}
	}
	for name, content := range after {
		if strings.HasPrefix(name, "api/") || name == "buf.yaml" || strings.Contains(name, "/port/rpc/") {
			t.Fatalf("unexpected contract %s", name)
		}
		if strings.HasSuffix(name, ".go") && (strings.Contains(content, "google.golang.org/grpc") || strings.Contains(content, "api/gen/go")) {
			t.Fatalf("protocol dependency in %s", name)
		}
	}
	if strings.Contains(after["Taskfile.yml"], "\n  api:\n") || strings.Contains(after[".github/workflows/ci.yml"], "contracts:") {
		t.Fatal("HTTP project requires contract gates")
	}
	code := after["services/order/internal/port/http/get_order.go"]
	if !strings.Contains(code, "OrderId string") || !strings.Contains(code, "GET /v1/orders/{order_id}") || !strings.Contains(code, "RegisterGetOrder") {
		t.Fatal(code)
	}
	if err := scaffold.AddAPI(t.Context(), dir, o); err == nil {
		t.Fatal("overwrote existing API")
	}
	if !reflect.DeepEqual(after, snapshot(t, dir)) {
		t.Fatal("failed addition changed project")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	o.Name = "next-order"
	if err := scaffold.AddAPI(ctx, dir, o); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, snapshot(t, dir)) {
		t.Fatal("canceled addition changed project")
	}
}

func TestNativeHTTPRejectsConflictsBeforeWriting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, file, code string }{
		{"method", "server.go", "package http\ntype OrderServer struct{}\nfunc (*OrderServer) GetOrder() {}"},
		{"field", "server.go", "package http\ntype OrderServer struct{ GetOrder string }"},
		{"request type", "other.go", "package http\ntype GetOrderRequest struct{}"},
		{"response type", "other.go", "package http\ntype GetOrderResponse struct{}"},
		{"request value", "other.go", "package http\nvar GetOrderRequest int"},
		{"registration", "other.go", "package http\nfunc RegisterGetOrder() {}"},
		{"route", "other.go", "package http\nimport h \"github.com/nostosx/servex-contrib/transport/httpx\"\nfunc mount(){h.RouteFunc(nil,\"GET /v1/orders/current\",nil)}"},
		{"dynamic route", "other.go", "package http\nimport h \"github.com/nostosx/servex-contrib/transport/httpx\"\nfunc mount(){h.RouteFunc(nil,pattern,nil)}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := httpProject(t)
			if err := os.WriteFile(filepath.Join(dir, "services/order/internal/port/http", tc.file), []byte(tc.code), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dir)
			err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "get-order", Service: "order", Method: "GET", Path: "/v1/orders/{order_id}"})
			if err == nil {
				t.Fatal("accepted conflict")
			}
			if !reflect.DeepEqual(before, snapshot(t, dir)) {
				t.Fatal("conflict changed project")
			}
		})
	}
}

func TestNativeAndGatewayRoutesCannotOverlap(t *testing.T) {
	t.Parallel()
	for _, first := range []string{"http", "gateway"} {
		t.Run(first, func(t *testing.T) {
			t.Parallel()
			dir := apiProject(t)
			op := scaffold.APIOptions{Context: "order", Name: "first-operation", Service: "order", Transport: first, Method: "GET", Path: "/v1/orders/{id}"}
			if err := scaffold.AddAPI(t.Context(), dir, op); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dir)
			op.Name = "second-operation"
			op.Transport = "gateway"
			if first == "gateway" {
				op.Transport = "http"
			}
			err := scaffold.AddAPI(t.Context(), dir, op)
			if err == nil || !strings.Contains(err.Error(), "overlap") {
				t.Fatalf("route conflict: %v", err)
			}
			if !reflect.DeepEqual(before, snapshot(t, dir)) {
				t.Fatal("route conflict changed project")
			}
		})
	}
}

func TestTransportSelection(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"http", "grpc", "http,grpc", "grpc,http"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			o := options(t)
			o.Transport = transport
			if _, err := scaffold.New(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"context", "service"} {
				if err := scaffold.Add(t.Context(), o.Output, kind, "order"); err != nil {
					t.Fatal(err)
				}
			}
			files := snapshot(t, o.Output)
			_, http := files["services/order/internal/port/http/server.go"]
			_, grpc := files["services/order/internal/port/rpc/server.go"]
			if http != (transport != "grpc") || grpc != strings.Contains(transport, "grpc") {
				t.Fatal("service did not inherit transports")
			}
			if !grpc {
				for _, mode := range []string{"grpc", "gateway"} {
					before := snapshot(t, o.Output)
					api := scaffold.APIOptions{Context: "order", Name: "ping", Service: "order", Transport: mode}
					if mode == "gateway" {
						api.Method = "GET"
						api.Path = "/ping"
					}
					if err := scaffold.AddAPI(t.Context(), o.Output, api); err == nil {
						t.Fatal("enabled missing transport")
					}
					if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
						t.Fatal("invalid transport changed project")
					}
				}
			}
		})
	}
	for _, transport := range []string{"udp", "http,http", "grpc,grpc", "http,grpc,other"} {
		o := options(t)
		o.Transport = transport
		if _, err := scaffold.New(t.Context(), o); err == nil {
			t.Fatalf("accepted %s", transport)
		}
		if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
			t.Fatal("invalid options created output")
		}
	}
}

func TestGeneratedTransportDependencies(t *testing.T) {
	t.Parallel()
	for _, layout := range []string{"monolith", "microservices"} {
		for _, agent := range []bool{false, true} {
			for _, transport := range []string{"http", "grpc", "http,grpc"} {
				name := layout + "/business/" + transport
				if agent {
					name = layout + "/agentx/" + transport
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					o := options(t)
					o.Layout, o.Agent, o.Transport = layout, agent, transport
					if _, err := scaffold.New(t.Context(), o); err != nil {
						t.Fatal(err)
					}
					files := snapshot(t, o.Output)
					wantHTTP := transport != "grpc"
					const backend = "github.com/nostosx/servex-contrib/transport/httpx"
					if strings.Contains(files["go.mod"], backend+" v1.0.0") != wantHTTP {
						t.Fatal("HTTP backend dependency does not match enabled transports")
					}
					if strings.Contains(files["go.mod"], "github.com/nostosx/servex-contrib/encoding/json v1.0.0 // indirect") != wantHTTP {
						t.Fatal("JSON dependency does not match enabled transports")
					}
					if !strings.Contains(files["go.mod"], "github.com/nostosx/servex-contrib/selector v1.2.0 // indirect") {
						t.Fatal("transport selector dependency is missing")
					}
					if !strings.Contains(files["go.mod"], "github.com/nostosx/servex-contrib/transport/client v1.0.0 // indirect") {
						t.Fatal("shared transport client dependency is missing")
					}
					if strings.Contains(files["go.mod"], "github.com/nostosx/servex-contrib/cache/loader") {
						t.Fatal("unused cache loader dependency is present")
					}
					if strings.Contains(files["infrastructure/server/server.go"], backend) != wantHTTP {
						t.Fatal("HTTP assembly does not use the selected backend module")
					}
					for path, content := range files {
						if strings.HasSuffix(path, ".go") && strings.Contains(content, "github.com/nostosx/servex/transport/httpx") {
							t.Fatalf("core HTTP implementation remains in %s", path)
						}
					}
				})
			}
		}
	}
}
