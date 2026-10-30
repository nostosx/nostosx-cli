package scaffold_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func apiProject(t *testing.T) string {
	t.Helper()
	o := options(t)
	o.Transport = "http,grpc"
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

func apiOptions() scaffold.APIOptions {
	return scaffold.APIOptions{Context: "order", Name: "create-order", Transport: "gateway", Service: "order", Method: "POST", Path: "/v1/orders/{order_id}"}
}

func TestAddAPI(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	protoPath := filepath.Join(dir, "api/proto/order/v1/order.proto")
	original := read(t, protoPath)
	// Byte offsets must preserve UTF-8, comments, string braces and CRLF input.
	original = strings.Replace(original, "service OrderService {}", "// 注释 { }\nservice OrderService {\n option deprecated = true; // }\n}\nmessage Other { string text = 1 [json_name = \"}\"]; }", 1)
	original = strings.ReplaceAll(original, "\n", "\r\n")
	if err := os.WriteFile(protoPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	if err := scaffold.AddAPI(t.Context(), dir, apiOptions()); err != nil {
		t.Fatal(err)
	}
	updated := read(t, protoPath)
	for _, preserved := range []string{"// 注释 { }\r\n", "option deprecated = true; // }\r\n", "message Other { string text = 1 [json_name = \"}\"]; }", "rpc CreateOrder(CreateOrderRequest) returns (CreateOrderResponse);\r\n"} {
		if !strings.Contains(updated, preserved) {
			t.Fatalf("missing preserved source or RPC %q: %s", preserved, updated)
		}
	}
	for name, content := range before {
		if name != "api/proto/order/v1/order.proto" && name != "services/order/app.go" && name != "services/order/internal/port/rpc/server.go" && name != "services/order/internal/application/command.go" && name != "services/order/internal/port/rpc/mapper.go" && read(t, filepath.Join(dir, name)) != content {
			t.Fatalf("changed existing %s", name)
		}
	}
	message := read(t, filepath.Join(dir, "api/proto/order/v1/create_order.proto"))
	if !strings.Contains(message, "string order_id = 1;") || strings.Contains(message, "Command") {
		t.Fatal(message)
	}
	if !strings.Contains(read(t, filepath.Join(dir, "api/http/routes/order_create_order.yaml")), "body: \"*\"") {
		t.Fatal("missing request body mapping")
	}
	// Multiple additions retain a valid service and use separate files.
	next := scaffold.APIOptions{Context: "order", Name: "get-order", Transport: "gateway", Service: "order", Method: "GET", Path: "/v1/orders/{id}"}
	if err := scaffold.AddAPI(t.Context(), dir, next); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, filepath.Join(dir, "api/http/routes/order_get_order.yaml")), "body:") {
		t.Fatal("GET received a body binding")
	}
	if len(snapshot(t, dir)) != len(before)+8 {
		t.Fatal("unexpected output count or temporary files left behind")
	}
}

func TestAPIValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, service, method, path string }{
		{"../escape", "order", "GET", "/orders"},
		{"create", "../escape", "GET", "/orders"},
		{"create", "order", "get", "/orders"},
		{"create", "order", "HEAD", "/orders"},
		{"create", "order", "GET", "/readyz"},
		{"create", "order", "GET", "/{anything}"},
		{"create", "order", "GET", "/orders/"},
		{"create", "order", "GET", "/orders//all"},
		{"create", "order", "GET", "/orders/../all"},
		{"create", "order", "GET", "/orders?all=true"},
		{"create", "order", "GET", "/orders/%2f"},
		{"create", "order", "GET", "/{name=orders/*}"},
		{"create", "order", "GET", "/{id}/{id}"},
		{"create", "order", "GET", "/{nested.field}"},
	} {
		o := scaffold.APIOptions{Context: "order", Name: tc.name, Transport: "gateway", Service: tc.service, Method: tc.method, Path: tc.path}
		if err := o.Validate(); err == nil {
			t.Errorf("accepted %+v", o)
		}
	}
}

func TestAPIKeepsImportComments(t *testing.T) {
	t.Parallel()
	for _, imported := range []string{"aaa", "zzz"} {
		t.Run(imported, func(t *testing.T) {
			t.Parallel()
			dir := apiProject(t)
			name := filepath.Join(dir, "api/proto/order/v1/order.proto")
			commentedImport := "// dependency documentation\nimport \"" + imported + ".proto\"; // dependency note"
			original := strings.Replace(read(t, name), "package order.v1;", commentedImport+"\n\npackage order.v1;", 1)
			if err := os.WriteFile(name, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := scaffold.AddAPI(t.Context(), dir, apiOptions()); err != nil {
				t.Fatal(err)
			}
			if updated := read(t, name); !strings.Contains(updated, commentedImport) {
				t.Fatalf("import comments changed association: %s", updated)
			}
		})
	}
}

func TestAPIConflictsLeaveProjectUnchanged(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, file, append, replace, with string }{
		{"RPC", "api/proto/order/v1/order.proto", "", "service OrderService {}", "service OrderService { rpc CreateOrder(OldRequest) returns (OldResponse); }"},
		{"message", "api/proto/order/v1/order.proto", "\nmessage CreateOrderRequest {}", "", ""},
		{"cross file symbol", "api/proto/order/v1/other.proto", "syntax=\"proto3\"; package order.v1; enum CreateOrderResponse { UNSPECIFIED=0; }", "", ""},
		{"invalid Proto", "api/proto/order/v1/order.proto", "\nmessage Broken {", "", ""},
		{"wrong package", "api/proto/order/v1/order.proto", "", "package order.v1;", "package renamed.v1;"},
		{"inline HTTP", "api/proto/order/v1/order.proto", "", "service OrderService {}", "service OrderService { rpc Other(OldRequest) returns (OldResponse) { option (google.api.http) = {get: \"/elsewhere\"}; }}"},
		{"adapter method", "services/order/internal/port/rpc/server.go", "\nfunc (*OrderServer) CreateOrder() {}", "", ""},
		{"adapter field", "services/order/internal/port/rpc/server.go", "", "struct {", "struct { CreateOrder string;"},
		{"route parameters", "api/http/routes/other.yaml", "- selector: other.v1.Other.Create\n  post: /v1/orders/{name}\n", "", ""},
		{"route literal overlap", "api/http/routes/other.yaml", "- selector: other.v1.Other.Create\n  post: /v1/orders/latest\n", "", ""},
		{"selector", "api/http/routes/other.yaml", "- selector: order.v1.OrderService.CreateOrder\n  get: /other\n", "", ""},
		{"additional binding", "api/http/routes/other.yaml", "- selector: other.v1.Other.Create\n  get: /other\n  additional_bindings:\n    - post: /v1/orders/{name}\n", "", ""},
		{"invalid YAML", "api/http/routes/other.yaml", "- {post: [", "", ""},
		{"multiple YAML docs", "api/http/routes/other.yaml", "[]\n---\n[]\n", "", ""},
		{"custom route", "api/http/routes/other.yaml", "- selector: other.v1.Other.Create\n  custom: {kind: HEAD, path: /other}\n", "", ""},
		{"existing output", "services/order/internal/port/rpc/create_order.go", "package rpc\n", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := apiProject(t)
			file := filepath.Join(dir, tc.file)
			content, err := os.ReadFile(file)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			content = []byte(strings.Replace(string(content), tc.replace, tc.with, 1) + tc.append)
			if err := os.WriteFile(file, content, 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dir)
			if err := scaffold.AddAPI(t.Context(), dir, apiOptions()); err == nil {
				t.Fatal("conflict accepted")
			}
			if !reflect.DeepEqual(before, snapshot(t, dir)) {
				t.Fatal("failure modified project")
			}
		})
	}
}

func TestAPILockCancellationAndRollback(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	before := snapshot(t, dir)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := scaffold.AddAPI(ctx, dir, apiOptions()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("cancelled API modified project")
	}
	lock := filepath.Join(dir, ".nostosx.lock")
	if err := os.WriteFile(lock, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.AddAPI(t.Context(), dir, apiOptions()); err == nil {
		t.Fatal("accepted held lock")
	}
	if read(t, lock) != "owned" {
		t.Fatal("removed another writer's lock")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	// Cancel after the first owned output is written to exercise partial rollback.
	ctx = cancelAfterOutput{Context: t.Context(), file: filepath.Join(dir, "api/http/routes/order_create_order.yaml")}
	if err := scaffold.AddAPI(ctx, dir, apiOptions()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("partial output was not rolled back")
	}
}

type cancelAfterOutput struct {
	context.Context
	file string
}

func (c cancelAfterOutput) Err() error {
	if _, err := os.Stat(c.file); err == nil {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestAPIConcurrentAdditions(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Go(func() { results <- scaffold.AddAPI(t.Context(), dir, apiOptions()) })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful additions=%d", success)
	}
	if strings.Count(read(t, filepath.Join(dir, "api/proto/order/v1/order.proto")), "rpc CreateOrder(") != 1 {
		t.Fatal("duplicate RPC")
	}
}

func TestAPIRejectsSymlinks(t *testing.T) {
	for _, name := range []string{"go.mod", "api/proto/order/v1/order.proto", "api/http/routes", "services/order/internal/port/rpc"} {
		t.Run(name, func(t *testing.T) {
			dir := apiProject(t)
			outside := t.TempDir()
			target := filepath.Join(dir, name)
			info, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() {
				outside = filepath.Join(outside, "original.proto")
				if err := os.WriteFile(outside, []byte(read(t, target)), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.RemoveAll(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, target); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if err := scaffold.AddAPI(t.Context(), dir, apiOptions()); err == nil {
				t.Fatal("accepted symlink")
			}
			if _, err := os.Lstat(filepath.Join(dir, ".nostosx.lock")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("lock left behind: %v", err)
			}
		})
	}
}

func TestAPIMethodBindings(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			dir := apiProject(t)
			o := apiOptions()
			o.Method = method
			if err := scaffold.AddAPI(t.Context(), dir, o); err != nil {
				t.Fatal(err)
			}
			binding := read(t, filepath.Join(dir, "api/http/routes/order_create_order.yaml"))
			wantBody := method == "POST" || method == "PUT" || method == "PATCH"
			if strings.Contains(binding, "body:") != wantBody || !strings.Contains(binding, strings.ToLower(method)+":") {
				t.Fatal(binding)
			}
		})
	}
}

func TestAPIRejectsOversizedInput(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	file := filepath.Join(dir, "api/http/routes/large.yaml")
	if err := os.WriteFile(file, []byte(strings.Repeat("#", (1<<20)+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	if err := scaffold.AddAPI(t.Context(), dir, apiOptions()); err == nil {
		t.Fatal("accepted oversized input")
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("oversized input modified project")
	}
}
