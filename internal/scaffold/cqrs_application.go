package scaffold

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

func serviceType(name string) string                { return strings.ToUpper(name[:1]) + name[1:] }
func useCaseAlias(o APIOptions, api apiData) string { return o.Context + api.Kind }

// Each service owns typed handler aggregates. Contexts own business requests and handlers.
func wireCQRS(ctx context.Context, root *os.Root, module string, o APIOptions, api apiData) ([]apiUpdate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	aggregate, err := wireApplication(root, module, o, api)
	if err != nil {
		return nil, err
	}
	composition, err := wireComposition(root, module, o, api)
	if err != nil {
		return nil, err
	}
	mapping, err := wireProtocolMapping(root, module, o, api)
	if err != nil {
		return nil, err
	}
	updates := []apiUpdate{aggregate, composition, mapping}
	if o.Transport == "" || o.Transport == "http" {
		registration, err := wireHTTPRegistration(root, o, api)
		if err != nil {
			return nil, err
		}
		updates = append(updates, registration)
	}
	return updates, nil
}

func wireApplication(root *os.Root, module string, o APIOptions, api apiData) (apiUpdate, error) {
	name := "services/" + o.Service + "/internal/application/" + api.Kind + ".go"
	return updateGo(root, name, func(fset *token.FileSet, f *ast.File, b []byte) ([]sourceEdit, error) {
		if f.Name.Name != "application" {
			return nil, errors.New("service handler aggregates must use package application")
		}
		alias := useCaseAlias(o, api)
		imports := map[string]string{"cqrs": "github.com/nostosx/servex/cqrs", alias: module + "/application/" + o.Context + "/" + api.Kind}
		edits, err := importEdits(fset, f, b, imports)
		if err != nil {
			return nil, err
		}
		typ := fmt.Sprintf("cqrs.%s[%s.%s%s, %s.%sResult]", api.KindType, alias, api.RPC, api.KindType, alias, api.RPC)
		existing := 0
		for _, structName := range []string{api.KindType, api.KindType + "Deps"} {
			structure := namedStruct(f, structName)
			if structure == nil {
				return nil, fmt.Errorf("service must declare %s as a non-generic struct", structName)
			}
			found := false
			for _, field := range structure.Fields.List {
				for _, n := range field.Names {
					if n.Name == api.Field {
						if len(field.Names) != 1 || expression(field.Type) != typ {
							return nil, fmt.Errorf("aggregate field %s has an incompatible context or type", api.Field)
						}
						found = true
						existing++
					}
				}
			}
			if !found {
				pos := fset.Position(structure.Fields.Closing).Offset
				edits = append(edits, sourceEdit{pos, pos, "\n" + api.Field + " " + typ + "\n"})
			}
		}
		constructor := namedFunction(f, "New"+api.KindType)
		if constructor == nil || constructor.Body == nil {
			return nil, errors.New("service aggregate constructor is missing")
		}
		if len(constructor.Type.Params.List) == 0 || len(constructor.Type.Params.List[0].Names) != 1 || constructor.Type.Params.List[0].Names[0].Name != "deps" {
			return nil, errors.New("aggregate constructor must accept its handler dependencies as deps")
		}
		literal, err := uniqueLiteral(constructor.Body, "", api.KindType)
		if err != nil {
			return nil, err
		}
		value, err := literalField(literal, api.Field)
		if err != nil {
			return nil, err
		}
		if value != nil {
			if existing != 2 || !cqrsChain(value, api) {
				return nil, errors.New("existing aggregate chain has incompatible operation metadata or dependencies")
			}
		} else {
			if existing != 0 {
				return nil, errors.New("existing aggregate field has no matching chain; wire customized handler aggregates manually")
			}
			value := fmt.Sprintf("cqrs.Chain%s(%q, deps.%s)", api.KindType, api.Operation, api.Field)
			edits = append(edits, literalInsertion(fset, literal, b, api.Field+": "+value))
		}
		return edits, nil
	})
}

func wireComposition(root *os.Root, module string, o APIOptions, api apiData) (apiUpdate, error) {
	return updateGo(root, "services/"+o.Service+"/app.go", func(fset *token.FileSet, f *ast.File, b []byte) ([]sourceEdit, error) {
		register := namedFunction(f, "Register")
		if register == nil || register.Body == nil {
			return nil, errors.New("service app.go must declare Register")
		}
		if err := requireImport(f, "serviceapp", module+"/services/"+o.Service+"/internal/application"); err != nil {
			return nil, err
		}
		literal, err := uniqueLiteral(register.Body, "serviceapp", api.KindType+"Deps")
		if err != nil {
			return nil, err
		}
		value, err := literalField(literal, api.Field)
		if err != nil {
			return nil, err
		}
		if value != nil {
			return nil, nil
		} // A second transport reuses the same constructed handler and chain.
		usecase := "application/" + o.Context + "/" + api.Kind + "/" + api.File + ".go"
		source, _, readErr := readAPIFile(root, usecase)
		if readErr == nil {
			declaration, err := parser.ParseFile(token.NewFileSet(), usecase, source, 0)
			if err != nil {
				return nil, err
			}
			constructor := namedFunction(declaration, "New"+api.RPC+"Handler")
			if constructor == nil || len(fieldTypes(constructor.Type.Params)) != 0 {
				return nil, errors.New("existing use case requires explicit constructor dependencies; wire it in the service manually")
			}
		} else if !errors.Is(readErr, fs.ErrNotExist) {
			return nil, readErr
		}
		alias := useCaseAlias(o, api)
		edits, err := importEdits(fset, f, b, map[string]string{alias: module + "/application/" + o.Context + "/" + api.Kind})
		if err != nil {
			return nil, err
		}
		edits = append(edits, literalInsertion(fset, literal, b, fmt.Sprintf("%s: %s.New%sHandler()", api.Field, alias, api.RPC)))
		return edits, nil
	})
}

func wireHTTPRegistration(root *os.Root, o APIOptions, api apiData) (apiUpdate, error) {
	name := "services/" + o.Service + "/internal/port/http/server.go"
	return updateGo(root, name, func(fset *token.FileSet, f *ast.File, _ []byte) ([]sourceEdit, error) {
		register := namedFunction(f, "Register")
		if register == nil || register.Body == nil {
			return nil, errors.New("HTTP adapter must declare Register")
		}
		params := register.Type.Params
		if len(params.List) != 2 || len(params.List[0].Names) != 1 || params.List[0].Names[0].Name != "server" || len(params.List[1].Names) != 1 || params.List[1].Names[0].Name != "handler" {
			return nil, errors.New("HTTP Register must accept server and handler parameters")
		}
		pos := fset.Position(register.Body.Rbrace).Offset
		return []sourceEdit{{pos, pos, fmt.Sprintf("\nRegister%s(server,handler)\n", api.RPC)}}, nil
	})
}

func updateGo(root *os.Root, name string, edit func(*token.FileSet, *ast.File, []byte) ([]sourceEdit, error)) (apiUpdate, error) {
	b, mode, err := readAPIFile(root, name)
	if err != nil {
		return apiUpdate{}, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, b, parser.ParseComments)
	if err != nil {
		return apiUpdate{}, err
	}
	if err := unconditionalSource(name, f); err != nil {
		return apiUpdate{}, err
	}
	edits, err := edit(fset, f, b)
	if err != nil {
		return apiUpdate{}, err
	}
	updated := b
	if len(edits) > 0 {
		updated, err = editGo(b, edits)
	}
	return apiUpdate{name, b, updated, mode}, err
}
func namedStruct(f *ast.File, name string) *ast.StructType {
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok {
			for _, s := range g.Specs {
				if t, ok := s.(*ast.TypeSpec); ok && t.Name.Name == name && t.TypeParams == nil {
					structure, _ := t.Type.(*ast.StructType)
					return structure
				}
			}
		}
	}
	return nil
}
func namedFunction(f *ast.File, name string) *ast.FuncDecl {
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}
func uniqueLiteral(body *ast.BlockStmt, pkg, name string) (*ast.CompositeLit, error) {
	var found *ast.CompositeLit
	count := 0
	ast.Inspect(body, func(n ast.Node) bool {
		literal, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		matches := false
		if pkg == "" {
			id, ok := literal.Type.(*ast.Ident)
			matches = ok && id.Name == name
		} else {
			sel, ok := literal.Type.(*ast.SelectorExpr)
			if ok {
				p, ok := sel.X.(*ast.Ident)
				matches = ok && p.Name == pkg && sel.Sel.Name == name
			}
		}
		if matches {
			found = literal
			count++
		}
		return true
	})
	if count != 1 {
		return nil, fmt.Errorf("assembly must construct exactly one %s literal; wire customized assembly manually", name)
	}
	return found, nil
}
func literalField(literal *ast.CompositeLit, name string) (ast.Expr, error) {
	var found ast.Expr
	for _, e := range literal.Elts {
		pair, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return nil, errors.New("assembly literals must use named fields")
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok {
			return nil, errors.New("assembly fields must use identifiers")
		}
		if key.Name == name {
			if found != nil {
				return nil, fmt.Errorf("duplicate assembly field %s", name)
			}
			found = pair.Value
		}
	}
	return found, nil
}
func literalInsertion(fset *token.FileSet, literal *ast.CompositeLit, b []byte, field string) sourceEdit {
	pos := fset.Position(literal.Rbrace).Offset
	prefix := ""
	if len(literal.Elts) > 0 {
		last := fset.Position(literal.Elts[len(literal.Elts)-1].End()).Offset
		if !strings.Contains(string(b[last:pos]), ",") {
			prefix = ","
		}
	}
	return sourceEdit{pos, pos, prefix + "\n" + field + ",\n"}
}
func expression(expr ast.Expr) string {
	var out bytes.Buffer
	if err := format.Node(&out, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return out.String()
}
func cqrsChain(expr ast.Expr, api apiData) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) < 2 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "cqrs" || sel.Sel.Name != "Chain"+api.KindType {
		return false
	}
	operation, ok := call.Args[0].(*ast.BasicLit)
	if !ok {
		return false
	}
	value, err := strconv.Unquote(operation.Value)
	return err == nil && value == api.Operation && expression(call.Args[1]) == "deps."+api.Field
}
func requireImport(f *ast.File, alias, wanted string) error {
	for _, i := range f.Imports {
		p, _ := strconv.Unquote(i.Path.Value)
		if p == wanted && i.Name != nil && i.Name.Name == alias {
			return nil
		}
	}
	return fmt.Errorf("service assembly must import %s as %s", wanted, alias)
}

// Keep protocol/application conversion together in the adapter mapper.
func wireProtocolMapping(root *os.Root, module string, o APIOptions, api apiData) (apiUpdate, error) {
	transport := "rpc"
	if o.Transport == "" || o.Transport == "http" {
		transport = "http"
	}
	name := "services/" + o.Service + "/internal/port/" + transport + "/mapper.go"
	return updateGo(root, name, func(fset *token.FileSet, f *ast.File, b []byte) ([]sourceEdit, error) {
		if f.Name.Name != transport {
			return nil, errors.New("mapper must use its protocol package")
		}
		for _, name := range []string{"map" + api.RPC + "Request", "map" + api.RPC + "Response"} {
			if f.Scope != nil && f.Scope.Lookup(name) != nil {
				return nil, fmt.Errorf("protocol mapping symbol %s already exists", name)
			}
		}
		alias := useCaseAlias(o, api)
		imports := map[string]string{alias: module + "/application/" + o.Context + "/" + api.Kind}
		prefix := ""
		if transport == "rpc" {
			prefix = "pb."
			imports["pb"] = module + "/api/gen/go/" + o.Service + "/v1"
		}
		edits, err := importEdits(fset, f, b, imports)
		if err != nil {
			return nil, err
		}
		var fields strings.Builder
		for _, field := range api.Fields {
			value := "request." + field.GoName
			if transport == "rpc" {
				value = "request.Get" + field.GoName + "()"
			}
			fmt.Fprintf(&fields, "%s: %s,\n", field.GoName, value)
		}
		functions := fmt.Sprintf("\nfunc map%sRequest(request *%s%sRequest) %s.%s%s { return %s.%s%s{%s} }\nfunc map%sResponse(_ %s.%sResult) *%s%sResponse {return &%s%sResponse{}}\n", api.RPC, prefix, api.RPC, alias, api.RPC, api.KindType, alias, api.RPC, api.KindType, fields.String(), api.RPC, alias, api.RPC, prefix, api.RPC, prefix, api.RPC)
		return append(edits, sourceEdit{len(b), len(b), functions}), nil
	})
}
