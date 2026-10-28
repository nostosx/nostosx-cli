package scaffold

import (
	"context"
	"errors"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"strings"

	"github.com/bufbuild/protocompile/ast"
	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
)

func extendContract(ctx context.Context, root *os.Root, target string, original []byte, module string, o APIOptions) ([]byte, error) {
	rpcName := o.rpcName()
	wantedPackage := o.Service + ".v1"
	serviceName := strings.ToUpper(o.Service[:1]) + o.Service[1:] + "Service"
	var targetFile *ast.FileNode
	var service *ast.ServiceNode
	err := scanAPIFiles(ctx, root, "api/proto", ".proto", func(name string, b []byte) error {
		file, err := parser.Parse(name, strings.NewReader(string(b)), reporter.NewHandler(nil))
		if err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		// External YAML and inline HTTP options can shadow each other. Refuse
		// mixed mapping sources instead of making an incomplete conflict check.
		if err := ast.Walk(file, &ast.NoOpVisitor{}, ast.WithBefore(func(n ast.Node) error {
			if o.Method == "" {
				return nil
			}
			if option, ok := n.(*ast.OptionNode); ok {
				for _, part := range option.Name.Parts {
					if strings.TrimPrefix(string(part.Name.AsIdentifier()), ".") == "google.api.http" {
						return fmt.Errorf("%s uses inline google.api.http; use external api/http/routes bindings before adding APIs", name)
					}
				}
			}
			return nil
		})); err != nil {
			return err
		}
		pkg, goPackage := "", ""
		for _, decl := range file.Decls {
			switch n := decl.(type) {
			case *ast.PackageNode:
				pkg = string(n.Name.AsIdentifier())
			case *ast.OptionNode:
				if file.NodeInfo(n.Name).RawText() == "go_package" {
					if value, ok := n.Val.(ast.StringValueNode); ok {
						goPackage = value.AsString()
					}
				}
			}
		}
		if name == target {
			if string(b) != string(original) {
				return errors.New("service Proto changed during generation")
			}
			if pkg != wantedPackage || goPackage != module+"/api/gen/go/"+o.Service+"/v1;"+o.Service+"v1" {
				return errors.New("service Proto package or go_package does not match the generated v1 layout")
			}
			if file.Syntax == nil || file.Syntax.Syntax.AsString() != "proto3" {
				return errors.New("add api requires a proto3 service")
			}
			targetFile = file
		}
		if pkg != wantedPackage {
			return nil
		}
		for _, decl := range file.Decls {
			symbol := ""
			switch n := decl.(type) {
			case *ast.MessageNode:
				symbol = n.Name.Val
			case *ast.EnumNode:
				symbol = n.Name.Val
				for _, value := range n.Decls {
					if v, ok := value.(*ast.EnumValueNode); ok && (v.Name.Val == rpcName+"Request" || v.Name.Val == rpcName+"Response") {
						return fmt.Errorf("proto enum value %s already exists in %s", v.Name.Val, name)
					}
				}
			case *ast.ServiceNode:
				symbol = n.Name.Val
				if n.Name.Val == serviceName {
					if name != target || service != nil {
						return fmt.Errorf("service %s has a conflicting declaration", serviceName)
					}
					service = n
					for _, element := range n.Decls {
						if method, ok := element.(*ast.RPCNode); ok && method.Name.Val == rpcName {
							return fmt.Errorf("RPC %s already exists", rpcName)
						}
					}
				}
			}
			if symbol == rpcName+"Request" || symbol == rpcName+"Response" {
				return fmt.Errorf("proto symbol %s already exists in %s", symbol, name)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if service == nil || targetFile == nil {
		return nil, fmt.Errorf("target Proto must declare %s", serviceName)
	}
	newline := "\n"
	if strings.Contains(string(original), "\r\n") {
		newline = "\r\n"
	}
	closeOffset := targetFile.NodeInfo(service.CloseBrace).Start().Offset
	lineStart := strings.LastIndex(string(original[:closeOffset]), "\n") + 1
	if strings.TrimSpace(string(original[lineStart:closeOffset])) == "" {
		closeOffset = lineStart
	}
	importPath := o.Service + "/v1/" + strings.ReplaceAll(o.Name, "-", "_") + ".proto"
	importOffset, importPrefix, err := contractImportPosition(targetFile, importPath, newline)
	if err != nil {
		return nil, err
	}
	if importOffset >= closeOffset {
		return nil, errors.New("proto imports and package must precede the target service")
	}
	method := fmt.Sprintf("%s  rpc %s(%sRequest) returns (%sResponse);%s", newline, rpcName, rpcName, rpcName, newline)
	// Insert from right to left so AST byte offsets remain valid, including UTF-8.
	updated := string(original[:closeOffset]) + method + string(original[closeOffset:])
	updated = updated[:importOffset] + importPrefix + "import " + fmt.Sprintf("%q;", importPath) + newline + updated[importOffset:]
	return []byte(updated), nil
}

// Preserve source formatting and keep imports ordered without printing the AST.
func contractImportPosition(file *ast.FileNode, name, newline string) (int, string, error) {
	offset, prefix := 0, ""
	selected := false
	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.PackageNode:
			if offset == 0 {
				offset = file.NodeInfo(node).Start().Offset
				if comments := file.NodeInfo(node).LeadingComments(); comments.Len() > 0 {
					offset = comments.Index(0).Start().Offset
				}
			}
		case *ast.ImportNode:
			if node.Name.AsString() == name {
				return 0, "", fmt.Errorf("proto import %s already exists", name)
			}
			if selected {
				continue
			}
			info := file.NodeInfo(node)
			if node.Name.AsString() > name {
				offset, prefix, selected = info.Start().Offset, "", true
				if comments := info.LeadingComments(); comments.Len() > 0 {
					offset = comments.Index(0).Start().Offset
				}
			} else {
				offset, prefix = info.Start().Offset+len(info.RawText()), newline
				if comments := info.TrailingComments(); comments.Len() > 0 {
					last := comments.Index(comments.Len() - 1)
					offset = last.Start().Offset + len(last.RawText())
				}
			}
		}
	}
	return offset, prefix, nil
}

func checkAdapter(ctx context.Context, root *os.Root, o APIOptions) error {
	return checkNamedAdapter(ctx, root, o.Service, o.rpcName())
}

func checkNamedAdapter(ctx context.Context, root *os.Root, service, rpc string) error {
	found := false
	returnErr := scanAPIFiles(ctx, root, "services/"+service+"/internal/port/rpc", ".go", func(name string, b []byte) error {
		isTest := strings.HasSuffix(name, "_test.go")
		file, err := goparser.ParseFile(token.NewFileSet(), name, b, 0)
		if err != nil {
			return fmt.Errorf("parse adapter %s: %w", name, err)
		}
		if isTest && file.Name.Name == "rpc_test" {
			return nil
		}
		if file.Name.Name != "rpc" {
			return fmt.Errorf("adapter %s must use package rpc", name)
		}
		for _, decl := range file.Decls {
			switch n := decl.(type) {
			case *goast.FuncDecl:
				if n.Recv != nil && receiverName(n.Recv.List[0].Type) == "Handler" && n.Name.Name == rpc {
					return fmt.Errorf("handler method %s already exists", rpc)
				}
			case *goast.GenDecl:
				for _, spec := range n.Specs {
					typeSpec, ok := spec.(*goast.TypeSpec)
					if !ok || typeSpec.Name.Name != "Handler" {
						continue
					}
					if typeSpec.TypeParams != nil {
						return errors.New("handler must not have type parameters")
					}
					structure, ok := typeSpec.Type.(*goast.StructType)
					if !ok {
						return errors.New("handler must be a struct")
					}
					found = true
					for _, field := range structure.Fields.List {
						if len(field.Names) == 0 && receiverName(field.Type) == rpc {
							return fmt.Errorf("handler embedded field conflicts with %s", rpc)
						}
						for _, fieldName := range field.Names {
							if fieldName.Name == rpc {
								return fmt.Errorf("handler field conflicts with %s", rpc)
							}
						}
					}
				}
			}
		}
		return nil
	})
	if returnErr != nil {
		return returnErr
	}
	if !found {
		return errors.New("existing service must declare a Handler struct")
	}
	return nil
}

func receiverName(expr goast.Expr) string {
	switch n := expr.(type) {
	case *goast.Ident:
		return n.Name
	case *goast.StarExpr:
		return receiverName(n.X)
	case *goast.SelectorExpr:
		return n.Sel.Name
	default:
		return ""
	}
}
