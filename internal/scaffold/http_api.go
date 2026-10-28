package scaffold

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

func addHTTPAPI(ctx context.Context, root *os.Root, module string, o APIOptions) error {
	rpc := o.rpcName()
	dir := "services/" + o.Service + "/internal/port/http"
	found := false
	if err := scanAPIFiles(ctx, root, dir, ".go", func(name string, b []byte) error {
		if path.Dir(name) != dir {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, b, 0)
		if err != nil {
			return fmt.Errorf("parse HTTP adapter %s: %w", name, err)
		}
		if strings.HasSuffix(name, "_test.go") && file.Name.Name == "http_test" {
			return nil
		}
		if file.Name.Name != "http" {
			return fmt.Errorf("adapter %s must use package http", name)
		}
		for _, decl := range file.Decls {
			switch n := decl.(type) {
			case *ast.FuncDecl:
				if n.Name.Name == "Register"+rpc || n.Recv != nil && receiverName(n.Recv.List[0].Type) == "Handler" && n.Name.Name == rpc {
					return fmt.Errorf("HTTP adapter %s already exists", n.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range n.Specs {
					if value, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range value.Names {
							if name.Name == rpc+"Request" || name.Name == rpc+"Response" || name.Name == "Register"+rpc {
								return fmt.Errorf("HTTP symbol %s already exists", name.Name)
							}
						}
					}
					if typ, ok := spec.(*ast.TypeSpec); ok {
						if typ.Name.Name == rpc+"Request" || typ.Name.Name == rpc+"Response" {
							return fmt.Errorf("HTTP type %s already exists", typ.Name.Name)
						}
						if typ.Name.Name == "Handler" {
							structure, ok := typ.Type.(*ast.StructType)
							if !ok || typ.TypeParams != nil {
								return errors.New("HTTP Handler must be a non-generic struct")
							}
							found = true
							for _, field := range structure.Fields.List {
								if len(field.Names) == 0 && receiverName(field.Type) == rpc {
									return fmt.Errorf("handler embedded field conflicts with %s", rpc)
								}
								for _, name := range field.Names {
									if name.Name == rpc {
										return fmt.Errorf("handler field conflicts with %s", rpc)
									}
								}
							}
						}
					}
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if !found {
		return errors.New("existing service must declare an HTTP Handler struct")
	}
	if err := checkHTTPRoutes(ctx, root, o); err != nil {
		return err
	}
	if _, err := root.Stat("api/http/routes"); err == nil {
		if err := checkRoutes(ctx, root, o); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	api := apiData{File: strings.ReplaceAll(o.Name, "-", "_"), RPC: rpc, Method: o.Method, Path: o.Path}
	parts, _ := routeSegments(o.Path)
	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			name := part[1 : len(part)-1]
			var field strings.Builder
			for p := range strings.SplitSeq(name, "_") {
				field.WriteString(strings.ToUpper(p[:1]) + p[1:])
			}
			api.Fields = append(api.Fields, apiField{Name: name, GoName: field.String()})
		}
	}
	files, err := render("http-api", data{Module: module, Service: o.Service, API: api})
	if err != nil {
		return err
	}
	return writeFiles(ctx, root, files)
}

// Inspect literal native route registrations across services. Dynamic routing
// remains application-owned; unsupported patterns require manual additions.
func checkHTTPRoutes(ctx context.Context, root *os.Root, o APIOptions) error {
	return scanAPIFiles(ctx, root, "services", ".go", func(name string, b []byte) error {
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, b, 0)
		if err != nil {
			return err
		}
		aliases := make(map[string]bool)
		for _, imp := range file.Imports {
			value, e := strconv.Unquote(imp.Path.Value)
			if e != nil {
				return e
			}
			if value != "github.com/nostosx/servex/transport/httpx" {
				continue
			}
			alias := "httpx"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			if alias == "." {
				return fmt.Errorf("cannot check dot-imported HTTP routes in %s", name)
			}
			aliases[alias] = true
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if err != nil {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fun := call.Fun
			switch indexed := fun.(type) {
			case *ast.IndexExpr:
				fun = indexed.X
			case *ast.IndexListExpr:
				fun = indexed.X
			}
			selector, ok := fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "RouteFunc" && selector.Sel.Name != "Route" {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok || !aliases[qualifier.Name] {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			literal, ok := call.Args[1].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				err = fmt.Errorf("cannot check dynamic HTTP route in %s; add this API manually", name)
				return false
			}
			pattern, e := strconv.Unquote(literal.Value)
			if e != nil {
				err = e
				return false
			}
			method, route, ok := strings.Cut(pattern, " ")
			if !ok {
				err = fmt.Errorf("cannot check HTTP pattern %q in %s", pattern, name)
				return false
			}
			if _, e := routeSegments(route); e != nil {
				err = fmt.Errorf("cannot check HTTP route in %s: %w", name, e)
				return false
			}
			if (method == o.Method || method == http.MethodGet && o.Method == http.MethodHead) && routesOverlap(route, o.Path) {
				err = fmt.Errorf("HTTP %s %s overlaps existing route %s", o.Method, o.Path, route)
				return false
			}
			return true
		})
		return err
	})
}
