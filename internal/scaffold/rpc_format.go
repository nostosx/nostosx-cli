package scaffold

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"strconv"
	"strings"
)

// protogen resolves Go identifiers and aliases. Group its imports to match the
// generated application's goimports configuration without executing a tool.
func formatRPCImports(source []byte, module string, packageNames map[string]string) ([]byte, error) {
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, "adapter.go", source, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var groups [3][]string
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		group := 1
		if !strings.Contains(strings.Split(value, "/")[0], ".") {
			group = 0
		}
		if value == module || strings.HasPrefix(value, module+"/") {
			group = 2
		}
		line := strconv.Quote(value)
		defaultName := path.Base(value)
		if declared, ok := packageNames[value]; ok {
			defaultName = declared
		}
		if spec.Name != nil && spec.Name.Name != defaultName {
			line = spec.Name.Name + " " + line
		}
		groups[group] = append(groups[group], line)
	}
	var imports strings.Builder
	imports.WriteString("import (\n")
	for _, group := range groups {
		if len(group) > 0 {
			imports.WriteString(strings.Join(group, "\n") + "\n\n")
		}
	}
	imports.WriteString(")")
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			start, end := positions.Position(gen.Pos()).Offset, positions.Position(gen.End()).Offset
			return format.Source([]byte(string(source[:start]) + imports.String() + string(source[end:])))
		}
	}
	return format.Source(source)
}
