package scaffold

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/pluginpb"

	"github.com/nostosx/nostosx-cli/internal/projectfs"
)

// ScaffoldRPC fills missing unary Handler methods from a local v1 contract.
// It never executes tools, rewrites implementations or creates business use cases.
// Imports must be local to api/proto or standard Protobuf well-known types.
func ScaffoldRPC(ctx context.Context, dir, service string) (created []string, err error) {
	if err := validateName(service, true); err != nil {
		return nil, err
	}
	root, release, err := projectfs.Lock(dir)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, release()) }()
	module, err := readModulePath(root)
	if err != nil {
		return nil, err
	}
	plugin, contract, err := compileService(ctx, root, module, service)
	if err != nil {
		return nil, err
	}
	adapter := "services/" + service + "/internal/port/rpc"
	packageNames := make(map[string]string)
	for _, file := range plugin.Files {
		packageNames[string(file.GoImportPath)] = string(file.GoPackageName)
	}
	methods, err := handlerMethods(ctx, root, adapter, packageNames)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte)
	names := make(map[string]bool)
	for _, method := range contract.Methods {
		if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
			return nil, fmt.Errorf("RPC %s is streaming; scaffold supports unary RPCs only", method.GoName)
		}
		name := adapter + "/" + methodFilename(method.GoName) + ".go"
		if names[name] {
			return nil, fmt.Errorf("RPC output filename collision: %s", name)
		}
		names[name] = true
		if existing, ok := methods[method.GoName]; ok {
			if !existing.matches(method) {
				return nil, fmt.Errorf("handler method %s has an incompatible signature", method.GoName)
			}
			continue
		}
		// Reuse field, package and Handler validation before generating a method.
		if err := checkNamedAdapter(ctx, root, service, method.GoName); err != nil {
			return nil, err
		}
		if err := realParents(root, name); err != nil {
			return nil, err
		}
		if _, err := root.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("RPC output already exists or is inaccessible: %s", name)
		}
		g := plugin.NewGeneratedFile(name, protogen.GoImportPath(module+"/"+adapter))
		g.P("package rpc")
		g.P("// ", method.GoName, " is the transport entry point. Implement authorization and the application call here.")
		g.P("func (*Handler) ", method.GoName, "(", protogen.GoIdent{GoName: "Context", GoImportPath: "context"}, ", *", method.Input.GoIdent, ") (*", method.Output.GoIdent, ", error) {")
		g.P("return nil, ", protogen.GoIdent{GoName: "Error", GoImportPath: "google.golang.org/grpc/status"}, "(", protogen.GoIdent{GoName: "Unimplemented", GoImportPath: "google.golang.org/grpc/codes"}, ", \"NOT_IMPLEMENTED\")")
		g.P("}")
		content, err := g.Content()
		if err != nil {
			return nil, err
		}
		content, err = formatRPCImports(content, module, packageNames)
		if err != nil {
			return nil, err
		}
		files[name] = content
		created = append(created, name)
	}
	if err := writeFiles(ctx, root, files); err != nil {
		return nil, err
	}
	return created, nil
}

func compileService(ctx context.Context, root *os.Root, module, service string) (*protogen.Plugin, *protogen.Service, error) {
	sources := make(map[string]string)
	if err := scanAPIFiles(ctx, root, "api/proto", ".proto", func(name string, b []byte) error {
		sources[strings.TrimPrefix(name, "api/proto/")] = string(b)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	target := service + "/v1/" + service + ".proto"
	compiler := protocompile.Compiler{Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(sources)})}
	compiled, err := compiler.Compile(ctx, target)
	if err != nil {
		return nil, nil, fmt.Errorf("compile local Proto: %w", err)
	}
	request := &pluginpb.CodeGeneratorRequest{FileToGenerate: []string{target}}
	seen := make(map[string]bool)
	var collect func(protoreflect.FileDescriptor)
	collect = func(file protoreflect.FileDescriptor) {
		if seen[file.Path()] {
			return
		}
		seen[file.Path()] = true
		for i := range file.Imports().Len() {
			collect(file.Imports().Get(i).FileDescriptor)
		}
		request.ProtoFile = append(request.ProtoFile, protodesc.ToFileDescriptorProto(file))
	}
	collect(compiled[0])
	plugin, err := (protogen.Options{}).New(request)
	if err != nil {
		return nil, nil, err
	}
	file := plugin.FilesByPath[target]
	if file.Desc.Syntax() != protoreflect.Proto3 || string(file.Desc.Package()) != service+".v1" || string(file.GoImportPath) != module+"/api/gen/go/"+service+"/v1" {
		return nil, nil, errors.New("service Proto must match the generated proto3 v1 package and go_package layout")
	}
	wanted := strings.ToUpper(service[:1]) + service[1:] + "Service"
	for _, candidate := range file.Services {
		if candidate.GoName == wanted {
			return plugin, candidate, nil
		}
	}
	return nil, nil, fmt.Errorf("target Proto must declare %s", wanted)
}

func methodFilename(name string) string {
	runes := []rune(name)
	var out strings.Builder
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) || i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
			out.WriteByte('_')
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}
