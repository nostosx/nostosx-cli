package scaffold

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"path"
	"slices"
	"strconv"
	"strings"
)

type sourceEdit struct {
	start, end int
	text       string
}

// Merge imports into one declaration while retaining import documentation.
func importEdits(fset *token.FileSet, f *ast.File, source []byte, wanted map[string]string) ([]sourceEdit, error) {
	type entry struct{ path, code string }
	var groups [3][]entry
	module := ""
	for alias, target := range wanted {
		if f.Scope != nil && f.Scope.Lookup(alias) != nil {
			return nil, fmt.Errorf("import alias %s conflicts with a declaration", alias)
		}
		if prefix, _, ok := strings.CutLast(target, "/application/"); ok {
			module = prefix
		}
	}
	group := func(target string) int {
		if module != "" && strings.HasPrefix(target, module+"/") {
			return 2
		}
		if strings.Contains(strings.Split(target, "/")[0], ".") {
			return 1
		}
		return 0
	}
	found := make(map[string]bool)
	for _, imp := range f.Imports {
		target, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		alias := path.Base(target)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		for name, wantedPath := range wanted {
			if target == wantedPath {
				if alias != name {
					return nil, fmt.Errorf("import %s must use alias %s for generated wiring", target, name)
				}
				found[name] = true
			} else if alias == name {
				return nil, fmt.Errorf("import alias %s already exists", name)
			}
		}
		text := string(source[fset.Position(imp.Pos()).Offset:fset.Position(imp.End()).Offset])
		if imp.Doc != nil {
			var comments []string
			for _, c := range imp.Doc.List {
				comments = append(comments, c.Text)
			}
			text = strings.Join(comments, "\n") + "\n" + text
		}
		if imp.Comment != nil {
			for _, c := range imp.Comment.List {
				text += " " + c.Text
			}
		}
		index := group(target)
		groups[index] = append(groups[index], entry{target, text})
	}
	for alias, target := range wanted {
		if !found[alias] {
			index := group(target)
			groups[index] = append(groups[index], entry{target, fmt.Sprintf("%s %q", alias, target)})
		}
	}
	var blocks []string
	for _, entries := range groups {
		if len(entries) == 0 {
			continue
		}
		slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.path, b.path) })
		var lines []string
		for _, e := range entries {
			lines = append(lines, e.code)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	declaration := "import (\n" + strings.Join(blocks, "\n\n") + "\n)"
	var edits []sourceEdit
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		start, end := fset.Position(gen.Pos()).Offset, fset.Position(gen.End()).Offset
		for _, spec := range gen.Specs {
			imp := spec.(*ast.ImportSpec)
			if imp.Comment != nil {
				end = max(end, fset.Position(imp.Comment.End()).Offset)
			}
		}
		replacement := ""
		if len(edits) == 0 {
			replacement = declaration
		}
		edits = append(edits, sourceEdit{start, end, replacement})
	}
	if len(edits) == 0 {
		pos := fset.Position(f.Name.End()).Offset
		edits = append(edits, sourceEdit{pos, pos, "\n\n" + declaration + "\n"})
	}
	return edits, nil
}
func editGo(b []byte, edits []sourceEdit) ([]byte, error) {
	slices.SortStableFunc(edits, func(a, b sourceEdit) int { return b.start - a.start })
	for _, e := range edits {
		b = append(append(append([]byte{}, b[:e.start]...), []byte(e.text)...), b[e.end:]...)
	}
	return format.Source(b)
}

func unconditionalSource(name string, file *ast.File) error {
	if platformFile(name) {
		return fmt.Errorf("%s is platform-specific; CQRS wiring requires unconditional source", name)
	}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build") || strings.HasPrefix(comment.Text, "// +build") {
				return fmt.Errorf("%s has build constraints; CQRS wiring requires unconditional source", name)
			}
		}
	}
	return nil
}
