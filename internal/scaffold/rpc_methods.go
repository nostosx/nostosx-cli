package scaffold

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

type handlerMethod struct {
	declaration *ast.FuncDecl
	imports     map[string]string
}

func handlerMethods(ctx context.Context, root *os.Root, dir string, packageNames map[string]string) (map[string]handlerMethod, error) {
	methods := make(map[string]handlerMethod)
	err := scanAPIFiles(ctx, root, dir, ".go", func(name string, b []byte) error {
		if path.Dir(name) != dir || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if platformFile(name) {
			return fmt.Errorf("adapter %s is platform-specific; scaffold requires unconditional Handler methods", name)
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, b, parser.ParseComments)
		if err != nil {
			return err
		}
		if file.Name.Name != "rpc" {
			return fmt.Errorf("adapter %s must use package rpc", name)
		}
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.HasPrefix(comment.Text, "//go:build") || strings.HasPrefix(comment.Text, "// +build") {
					return fmt.Errorf("adapter %s has build constraints; scaffold requires unconditional Handler methods", name)
				}
			}
		}
		imports := make(map[string]string)
		for _, spec := range file.Imports {
			value, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			alias := path.Base(value)
			if declared, ok := packageNames[value]; ok {
				alias = declared
			}
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			imports[alias] = value
		}
		for _, decl := range file.Decls {
			method, ok := decl.(*ast.FuncDecl)
			if !ok || method.Recv == nil || receiverName(method.Recv.List[0].Type) != "Handler" {
				continue
			}
			if _, exists := methods[method.Name.Name]; exists {
				return fmt.Errorf("duplicate Handler.%s", method.Name.Name)
			}
			methods[method.Name.Name] = handlerMethod{declaration: method, imports: imports}
		}
		return nil
	})
	return methods, err
}

func platformFile(name string) bool {
	parts := strings.Split(strings.TrimSuffix(path.Base(name), ".go"), "_")
	platforms := " aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos 386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm "
	return len(parts) > 1 && strings.Contains(platforms, " "+parts[len(parts)-1]+" ")
}

func (h handlerMethod) matches(method *protogen.Method) bool {
	params, results := fieldTypes(h.declaration.Type.Params), fieldTypes(h.declaration.Type.Results)
	if len(params) != 2 || len(results) != 2 {
		return false
	}
	return h.isType(params[0], protogen.GoIdent{GoName: "Context", GoImportPath: "context"}, false) &&
		h.isType(params[1], method.Input.GoIdent, true) && h.isType(results[0], method.Output.GoIdent, true) &&
		isError(results[1])
}

func fieldTypes(fields *ast.FieldList) []ast.Expr {
	var types []ast.Expr
	if fields != nil {
		for _, field := range fields.List {
			for range max(1, len(field.Names)) {
				types = append(types, field.Type)
			}
		}
	}
	return types
}

func (h handlerMethod) isType(expr ast.Expr, want protogen.GoIdent, pointer bool) bool {
	if pointer {
		star, ok := expr.(*ast.StarExpr)
		if !ok {
			return false
		}
		expr = star.X
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != want.GoName {
		return false
	}
	alias, ok := selector.X.(*ast.Ident)
	return ok && h.imports[alias.Name] == string(want.GoImportPath)
}

func isError(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
}
