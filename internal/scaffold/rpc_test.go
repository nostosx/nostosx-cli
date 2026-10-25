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

func TestAddGRPCOnlyAPI(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	before := snapshot(t, dir)
	if err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Name: "ping", Service: "order"}); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, dir)
	if len(after) != len(before)+3 {
		t.Fatalf("expected three new files, got %d", len(after)-len(before))
	}
	for name, content := range before {
		if strings.HasPrefix(name, "api/http/") && after[name] != content {
			t.Fatalf("changed HTTP configuration: %s", name)
		}
	}
	for _, options := range []scaffold.APIOptions{
		{Name: "pong", Service: "order", Method: "GET"},
		{Name: "pong", Service: "order", Path: "/pong"},
	} {
		if err := scaffold.AddAPI(t.Context(), dir, options); err == nil {
			t.Fatal("accepted incomplete HTTP binding")
		}
		if !reflect.DeepEqual(after, snapshot(t, dir)) {
			t.Fatal("validation changed project")
		}
	}
}

func rpcContract(t *testing.T, dir, methods string) {
	t.Helper()
	name := filepath.Join(dir, "api/proto/order/v1/order.proto")
	code := `syntax = "proto3";
package order.v1;
option go_package = "example.com/acme/shop/api/gen/go/order/v1;orderv1";
import "google/protobuf/empty.proto";
message Envelope { message Payload {} }
service OrderService { ` + methods + ` }
`
	// Match the module chosen by the common project fixture.
	module := read(t, filepath.Join(dir, "go.mod"))
	module = strings.Fields(module)[1]
	code = strings.ReplaceAll(code, "example.com/acme/shop", module)
	if err := os.WriteFile(name, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScaffoldRPCPreservesImplementations(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	rpcContract(t, dir, "rpc Fetch(Envelope.Payload) returns (google.protobuf.Empty); rpc Ping(google.protobuf.Empty) returns (Envelope.Payload);")
	before := snapshot(t, dir)
	created, err := scaffold.ScaffoldRPC(t.Context(), dir, "order")
	if err != nil || len(created) != 2 {
		t.Fatalf("created %v: %v", created, err)
	}
	code := read(t, filepath.Join(dir, created[0]))
	if !strings.Contains(code, "Envelope_Payload") || !strings.Contains(code, "emptypb.Empty") {
		t.Fatal(code)
	}
	for name, content := range before {
		if read(t, filepath.Join(dir, name)) != content {
			t.Fatalf("modified %s", name)
		}
	}
	// Preserve a real implementation, including a different context import alias.
	module := strings.Fields(read(t, filepath.Join(dir, "go.mod")))[1]
	code = `package rpc
import (
 ctx "context"
 "` + module + `/api/gen/go/order/v1"
 "google.golang.org/protobuf/types/known/emptypb"
)
func (*Handler) Fetch(ctx.Context, *orderv1.Envelope_Payload) (*emptypb.Empty, error) {
 return &emptypb.Empty{}, nil
}
`
	if err := os.WriteFile(filepath.Join(dir, created[0]), []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, dir)
	created, err = scaffold.ScaffoldRPC(t.Context(), dir, "order")
	if err != nil || len(created) != 0 || !reflect.DeepEqual(after, snapshot(t, dir)) {
		t.Fatalf("idempotence failed: %v %v", created, err)
	}
}

func TestScaffoldRPCRejectsUnsafeInputsBeforeWriting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, methods, file, code string }{
		{"stream", "rpc Fetch(google.protobuf.Empty) returns (stream Envelope.Payload);", "", ""},
		{"syntax", "invalid", "", ""},
		{"filename", "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);", "fetch.go", "package rpc\n"},
		{"signature", "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);", "existing.go", "package rpc\nfunc (*Handler) Fetch() {}\n"},
		{"field", "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);", "handler.go", "package rpc\ntype Handler struct {Fetch string}\n"},
		{"build constraints", "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);", "existing.go", "//go:build windows\n\npackage rpc\n"},
		{"platform file", "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);", "existing_windows.go", "package rpc\n"},
		{"generic handler", "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);", "handler.go", "package rpc\ntype Handler[T any] struct {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := apiProject(t)
			rpcContract(t, dir, tc.methods)
			if tc.file != "" {
				if err := os.WriteFile(filepath.Join(dir, "services/order/internal/port/rpc", tc.file), []byte(tc.code), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshot(t, dir)
			if _, err := scaffold.ScaffoldRPC(t.Context(), dir, "order"); err == nil {
				t.Fatal("unsafe input accepted")
			}
			if !reflect.DeepEqual(before, snapshot(t, dir)) {
				t.Fatal("failed operation changed project")
			}
		})
	}
}

func TestScaffoldRPCCancellationAndConcurrentWriters(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	rpcContract(t, dir, "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);")
	before := snapshot(t, dir)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := scaffold.ScaffoldRPC(ctx, dir, "order"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("cancellation changed project")
	}
	var group sync.WaitGroup
	results := make(chan int, 8)
	for range 8 {
		group.Go(func() {
			created, err := scaffold.ScaffoldRPC(t.Context(), dir, "order")
			if err == nil {
				results <- len(created)
			}
		})
	}
	group.Wait()
	close(results)
	total := 0
	for count := range results {
		total += count
	}
	if total != 1 {
		t.Fatalf("generated %d methods", total)
	}
	if len(snapshot(t, dir)) != len(before)+1 {
		t.Fatal("unexpected outputs or leaked lock")
	}
}

func TestScaffoldRPCRejectsSymlinks(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	rpcContract(t, dir, "rpc Fetch(google.protobuf.Empty) returns (Envelope.Payload);")
	name := filepath.Join(dir, "api/proto/order/v1/alias.proto")
	if err := os.Symlink(filepath.Join(t.TempDir(), "outside.proto"), name); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := scaffold.ScaffoldRPC(t.Context(), dir, "order"); err == nil {
		t.Fatal("accepted symlink")
	}
}
