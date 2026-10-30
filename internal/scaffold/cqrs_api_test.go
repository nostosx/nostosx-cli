package scaffold_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func TestAPIKindSelection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, method, kind, want string }{
		{"read", "GET", "", "query"}, {"write", "POST", "", "command"}, {"override", "POST", "query", "query"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := httpProject(t)
			err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "inspect-order", Service: "order", Method: tc.method, Path: "/orders", Kind: tc.kind})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "application/order", tc.want, "inspect_order.go")); err != nil {
				t.Fatal(err)
			}
			app := read(t, filepath.Join(dir, "services/order/internal/application", tc.want+".go"))
			if !strings.Contains(app, `"order.inspect_order"`) {
				t.Fatal(app)
			}
		})
	}
	for _, o := range []scaffold.APIOptions{
		{Context: "order", Name: "ping", Service: "order", Transport: "grpc"},
		{Context: "order", Name: "ping", Service: "order", Transport: "grpc", Kind: "event"},
	} {
		if err := o.Validate(); err == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
}

func TestAPIReusesApplicationAcrossTransports(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	http := scaffold.APIOptions{Context: "order", Name: "get-order", Service: "order", Method: "GET", Path: "/orders/{order_id}"}
	if err := scaffold.AddAPI(t.Context(), dir, http); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, "application/order/query/get_order.go")
	// Business implementation and comments belong to the application owner.
	body := strings.Replace(read(t, name), "operation is not implemented", "custom application behavior", 1)
	if err := os.WriteFile(name, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "get-order", Service: "order", Transport: "grpc", Kind: "query"}); err != nil {
		t.Fatal(err)
	}
	if read(t, name) != body {
		t.Fatal("rewrote existing business behavior")
	}
	app := read(t, filepath.Join(dir, "services/order/internal/application/query.go"))
	if strings.Count(app, "cqrs.ChainQuery(") != 1 || !strings.Contains(app, "deps.GetOrder") {
		t.Fatal(app)
	}
	proto := read(t, filepath.Join(dir, "api/proto/order/v1/get_order.proto"))
	if !strings.Contains(proto, "string order_id = 1;") {
		t.Fatal(proto)
	}
	before := snapshot(t, dir)
	if err := scaffold.AddAPI(t.Context(), dir, http); err == nil {
		t.Fatal("accepted duplicate API")
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("duplicate changed project")
	}
}

func TestCQRSConflictsLeaveProjectUnchanged(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, file, code string }{
		{"application symbol", "application/order/query/other.go", "package query;type GetOrderQuery struct{}"},
		{"aggregate dependency", "services/order/internal/application/query.go", "package application;type Query struct {GetOrder string};type QueryDeps struct{};func NewQuery(deps QueryDeps)*Query{return &Query{}}"},
		{"conditional server", "services/order/internal/port/http/server.go", "//go:build windows\n\npackage http;type OrderServer struct{}"},
		{"mapper symbol", "services/order/internal/port/http/mapper.go", "package http;var mapGetOrderRequest int"},
		{"custom assembly", "services/order/app.go", "package order;func Register(){}"},
		{"custom aggregate", "services/order/internal/application/query.go", "package application;type Query struct{};type QueryDeps struct{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := httpProject(t)
			code := tc.code
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(code), 0o600); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dir)
			err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "get-order", Service: "order", Method: "GET", Path: "/orders/{id}"})
			if err == nil {
				t.Fatal("accepted conflict")
			}
			if !reflect.DeepEqual(before, snapshot(t, dir)) {
				t.Fatal("conflict changed project")
			}
		})
	}
}

type cancelAfterReplacement struct {
	context.Context
	file, marker string
}

func (c cancelAfterReplacement) Err() error {
	b, err := os.ReadFile(c.file)
	if err == nil && strings.Contains(string(b), c.marker) {
		return context.Canceled
	}
	return c.Context.Err()
}
func TestCQRSRollbackRestoresReplacedFiles(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	before := snapshot(t, dir)
	name := filepath.Join(dir, "services/order/internal/application/command.go")
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	ctx := cancelAfterReplacement{t.Context(), name, "CreateOrderCommand"}
	if err := scaffold.AddAPI(ctx, dir, apiOptions()); !strings.Contains(errString(err), "canceled") {
		t.Fatalf("got %v", err)
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("rollback changed original bytes")
	}
	after, err := os.Stat(name)
	if err != nil || after.Mode() != info.Mode() {
		t.Fatalf("permissions changed: %v", err)
	}
}
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCQRSWiringPreservesImportCommentsAndPermissions(t *testing.T) {
	t.Parallel()
	dir := httpProject(t)
	name := filepath.Join(dir, "services/order/app.go")
	dependency := `"example.com/acme/shop/infrastructure/config"`
	code := strings.Replace(read(t, name), dependency, "// Transport dependency documentation.\n"+dependency+" // keep transport comment", 1)
	if err := os.WriteFile(name, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "get-order", Service: "order", Method: "GET", Path: "/orders"}); err != nil {
		t.Fatal(err)
	}
	updated := read(t, name)
	for _, comment := range []string{"// Transport dependency documentation.", "// keep transport comment"} {
		if strings.Count(updated, comment) != 1 {
			t.Fatal(updated)
		}
	}
	after, err := os.Stat(name)
	if err != nil || after.Mode() != before.Mode() {
		t.Fatalf("permissions changed: %v", err)
	}
}

func TestCQRSReusesGRPCUseCaseForHTTP(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	if err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "inspect-order", Service: "order", Transport: "grpc", Kind: "query"}); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.AddAPI(t.Context(), dir, scaffold.APIOptions{Context: "order", Name: "inspect-order", Service: "order", Method: "GET", Path: "/orders"}); err != nil {
		t.Fatal(err)
	}
	if app := read(t, filepath.Join(dir, "services/order/internal/application/query.go")); strings.Count(app, "cqrs.ChainQuery(") != 1 {
		t.Fatal(app)
	}
}

func TestServiceAndContextAreIndependent(t *testing.T) {
	t.Parallel()
	o := options(t)
	o.Transport = "http,grpc"
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Add(t.Context(), o.Output, "service", "studio"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(o.Output, "application/studio")); !os.IsNotExist(err) {
		t.Fatal("service created an implicit business context")
	}
	for _, name := range []string{"creation", "publishing"} {
		if err := scaffold.Add(t.Context(), o.Output, "context", name); err != nil {
			t.Fatal(err)
		}
	}
	api := scaffold.APIOptions{Service: "studio", Context: "creation", Name: "get-profile", Method: "GET", Path: "/profiles/{id}"}
	if err := scaffold.AddAPI(t.Context(), o.Output, api); err != nil {
		t.Fatal(err)
	}
	api.Transport = "grpc"
	api.Kind = "query"
	api.Method = ""
	api.Path = ""
	if err := scaffold.AddAPI(t.Context(), o.Output, api); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.AddAPI(t.Context(), o.Output, scaffold.APIOptions{Service: "studio", Context: "publishing", Name: "publish-document", Method: "POST", Path: "/publications"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"application/creation/query/get_profile.go", "application/creation/port/doc.go", "application/publishing/command/publish_document.go", "services/studio/internal/adapter/doc.go", "services/studio/internal/port/http/mapper.go"} {
		if _, err := os.Stat(filepath.Join(o.Output, name)); err != nil {
			t.Fatal(err)
		}
	}
	querySource := read(t, filepath.Join(o.Output, "services/studio/internal/application/query.go"))
	commandSource := read(t, filepath.Join(o.Output, "services/studio/internal/application/command.go"))
	app := read(t, filepath.Join(o.Output, "services/studio/app.go"))
	if !strings.Contains(querySource, `"creation.get_profile"`) || !strings.Contains(commandSource, `"publishing.publish_document"`) || strings.Contains(app, "cqrs.Chain") {
		t.Fatalf("incorrect ownership or wiring\n%s\n%s\n%s", querySource, commandSource, app)
	}
	before := snapshot(t, o.Output)
	api.Context = "publishing"
	api.Name = "get-profile"
	api.Transport = "gateway"
	api.Method = "GET"
	api.Path = "/other-profiles/{id}"
	if err := scaffold.AddAPI(t.Context(), o.Output, api); err == nil {
		t.Fatal("accepted one RPC name with conflicting contexts")
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("conflicting context changed project")
	}
}

func TestAPIRequiresExplicitContext(t *testing.T) {
	t.Parallel()
	o := scaffold.APIOptions{Service: "studio", Name: "get-profile", Method: "GET", Path: "/profiles"}
	if err := o.Validate(); err == nil {
		t.Fatal("accepted implicit context")
	}
	o.Context = "../creation"
	if err := o.Validate(); err == nil {
		t.Fatal("accepted escaping context")
	}
}

func TestSharedUseCasePreservesConstructorDependencies(t *testing.T) {
	t.Parallel()
	dir := apiProject(t)
	api := scaffold.APIOptions{Context: "order", Service: "order", Name: "get-order", Method: "GET", Path: "/orders/{id}"}
	if err := scaffold.AddAPI(t.Context(), dir, api); err != nil {
		t.Fatal(err)
	}
	handler := filepath.Join(dir, "application/order/query/get_order.go")
	source := strings.Replace(read(t, handler), "func NewGetOrderHandler()", "func NewGetOrderHandler(dependency string)", 1)
	if err := os.WriteFile(handler, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "services/order/app.go")
	source = strings.Replace(read(t, app), "orderquery.NewGetOrderHandler()", `orderquery.NewGetOrderHandler("configured")`, 1)
	if err := os.WriteFile(app, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	rpc := scaffold.APIOptions{Context: "order", Service: "order", Name: "get-order", Transport: "grpc", Kind: "query"}
	if err := scaffold.AddAPI(t.Context(), dir, rpc); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, app), `orderquery.NewGetOrderHandler("configured")`) {
		t.Fatal("rewrote constructor dependencies")
	}
	if err := scaffold.Add(t.Context(), dir, "service", "other"); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)
	api.Service = "other"
	api.Path = "/other-orders/{id}"
	if err := scaffold.AddAPI(t.Context(), dir, api); err == nil || !strings.Contains(err.Error(), "constructor dependencies") {
		t.Fatalf("expected dependency conflict, got %v", err)
	}
	if !reflect.DeepEqual(before, snapshot(t, dir)) {
		t.Fatal("dependency conflict modified project")
	}
}
