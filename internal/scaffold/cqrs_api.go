package scaffold

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"strings"
)

func (o APIOptions) kind() string {
	if o.Kind != "" {
		return o.Kind
	}
	if o.Method == http.MethodGet {
		return "query"
	}
	return "command"
}

func cqrsData(o APIOptions) apiData {
	kind := o.kind()
	api := apiData{File: strings.ReplaceAll(o.Name, "-", "_"), RPC: o.rpcName(), Method: o.Method, Path: o.Path, Kind: kind,
		KindType: strings.ToUpper(kind[:1]) + kind[1:], Operation: o.Context + "." + strings.ReplaceAll(o.Name, "-", "_")}
	api.Field = api.RPC
	parts, _ := routeSegments(o.Path)
	for _, part := range parts {
		if strings.HasPrefix(part, "{") {
			name := part[1 : len(part)-1]
			var field strings.Builder
			for p := range strings.SplitSeq(name, "_") {
				field.WriteString(strings.ToUpper(p[:1]) + p[1:])
			}
			api.Fields = append(api.Fields, apiField{Name: name, GoName: field.String(), Number: len(api.Fields) + 1})
		}
	}
	return api
}

// Prepare the entire change before publishing any file. Existing use cases can
// be shared by another transport, but their types and operation must agree.
func addCQRSAPI(ctx context.Context, root *os.Root, module string, o APIOptions, files map[string][]byte, proto string, before, after []byte, mode fs.FileMode) error {
	api := cqrsData(o)
	usecase := "application/" + o.Context + "/" + api.Kind + "/" + api.File + ".go"
	opposite := "command"
	if api.Kind == opposite {
		opposite = "query"
	}
	if _, err := root.Lstat("application/" + o.Context + "/" + opposite + "/" + api.File + ".go"); !errors.Is(err, fs.ErrNotExist) {
		return errors.New("API already has a use case of another kind")
	}
	existing, _, err := readAPIFile(root, usecase)
	if err == nil {
		fields, err := sharedUseCase(existing, api)
		if err != nil {
			return err
		}
		if len(api.Fields) > 0 && !sameFields(api.Fields, fields) {
			return errors.New("API path fields do not match the existing application input")
		}
		api.Fields = fields
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	d := data{Module: module, Service: o.Service, Context: o.Context, ServiceType: serviceType(o.Service), API: api}
	generated, err := render("usecase", d)
	if err != nil {
		return err
	}
	if existing == nil {
		if err := checkUseCaseSymbols(ctx, root, o.Context, api); err != nil {
			return err
		}
		maps.Copy(files, generated)
	}
	group := "api"
	if o.Transport == "" || o.Transport == "http" {
		group = "http-api"
	}
	// The same application input supplies both transport contracts.
	rendered, err := render(group, d)
	if err != nil {
		return err
	}
	for name, b := range rendered {
		if strings.HasSuffix(name, ".yaml") {
			continue
		}
		files[name] = b
	}
	if _, _, err := readAPIFile(root, "application/"+o.Context+"/doc.go"); err != nil {
		return fmt.Errorf("API requires an existing business context: %w", err)
	}
	updates, err := wireCQRS(ctx, root, module, o, api)
	if err != nil {
		return err
	}
	if proto != "" {
		updates = append(updates, apiUpdate{proto, before, after, mode})
	}
	return publishAPI(ctx, root, files, updates)
}

func sameFields(a, b []apiField) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].GoName != b[i].GoName {
			return false
		}
	}
	return true
}

func sharedUseCase(source []byte, api apiData) ([]apiField, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "usecase.go", source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	if err := unconditionalSource("usecase.go", file); err != nil {
		return nil, err
	}
	if file.Name.Name != api.Kind {
		return nil, errors.New("existing use case has an incompatible package")
	}
	types := map[string]*ast.StructType{}
	handler := false
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && receiverName(fn.Recv.List[0].Type) == api.RPC+"Handler" && fn.Name.Name == "Handle" {
			handler = validUseCaseSignature(fn, api)
		}
		if gen, ok := decl.(*ast.GenDecl); ok {
			for _, spec := range gen.Specs {
				if t, ok := spec.(*ast.TypeSpec); ok {
					if s, ok := t.Type.(*ast.StructType); ok && t.TypeParams == nil {
						types[t.Name.Name] = s
					}
				}
			}
		}
	}
	input, result := types[api.RPC+api.KindType], types[api.RPC+"Result"]
	if input == nil || result == nil || types[api.RPC+"Handler"] == nil || !handler || len(result.Fields.List) != 0 {
		return nil, errors.New("existing application use case is incompatible; add the transport manually after customizing its types")
	}
	var fields []apiField
	for _, f := range input.Fields.List {
		typ, ok := f.Type.(*ast.Ident)
		if !ok || typ.Name != "string" || len(f.Names) != 1 {
			return nil, errors.New("existing application input requires manual transport mapping")
		}
		name := f.Names[0].Name
		if !ast.IsExported(name) {
			return nil, errors.New("existing application input requires exported fields")
		}
		var snake strings.Builder
		for i, c := range name {
			if c >= 'A' && c <= 'Z' {
				if i > 0 {
					snake.WriteByte('_')
				}
				snake.WriteRune(c + 'a' - 'A')
			} else {
				snake.WriteRune(c)
			}
		}
		if !parameterSegment.MatchString("{" + snake.String() + "}") {
			return nil, errors.New("existing application field requires manual transport mapping")
		}
		fields = append(fields, apiField{Name: snake.String(), GoName: name, Number: len(fields) + 1})
	}
	return fields, nil
}

func checkUseCaseSymbols(ctx context.Context, root *os.Root, service string, api apiData) error {
	return scanAPIFiles(ctx, root, "application/"+service+"/"+api.Kind, ".go", func(name string, b []byte) error {
		file, err := parser.ParseFile(token.NewFileSet(), name, b, 0)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, "_test.go") && file.Name.Name == api.Kind+"_test" {
			return nil
		}
		if file.Name.Name != api.Kind {
			return fmt.Errorf("unexpected application package in %s", name)
		}
		for _, decl := range file.Decls {
			var names []string
			switch n := decl.(type) {
			case *ast.FuncDecl:
				if n.Recv == nil {
					names = append(names, n.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range n.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						names = append(names, s.Name.Name)
					case *ast.ValueSpec:
						for _, v := range s.Names {
							names = append(names, v.Name)
						}
					}
				}
			}
			for _, name := range names {
				if name == api.RPC+api.KindType || name == api.RPC+"Result" || name == api.RPC+"Handler" || name == "New"+api.RPC+"Handler" {
					return fmt.Errorf("application symbol %s already exists", name)
				}
			}
		}
		return nil
	})
}

func validUseCaseSignature(fn *ast.FuncDecl, api apiData) bool {
	flatten := func(fields *ast.FieldList) []ast.Expr {
		var types []ast.Expr
		if fields == nil {
			return types
		}
		for _, field := range fields.List {
			for range max(1, len(field.Names)) {
				types = append(types, field.Type)
			}
		}
		return types
	}
	params, results := flatten(fn.Type.Params), flatten(fn.Type.Results)
	if len(params) != 2 || len(results) != 2 || fn.Type.TypeParams != nil {
		return false
	}
	ctx, ok := params[0].(*ast.SelectorExpr)
	if !ok || ctx.Sel.Name != "Context" {
		return false
	}
	name, ok := ctx.X.(*ast.Ident)
	if !ok || name.Name != "context" {
		return false
	}
	ident := func(expr ast.Expr, want string) bool { value, ok := expr.(*ast.Ident); return ok && value.Name == want }
	return ident(params[1], api.RPC+api.KindType) && ident(results[0], api.RPC+"Result") && ident(results[1], "error")
}
