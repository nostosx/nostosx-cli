package contract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func configuration(ctx context.Context, root *os.Root, protos map[string][]byte) ([]byte, []byte, error) {
	rules, err := fragments(ctx, root, "api/http/routes")
	if err != nil {
		return nil, nil, err
	}
	methods, err := fragments(ctx, root, "api/http/operations")
	if err != nil {
		return nil, nil, err
	}
	common, err := readFile(root, "api/http/openapi.yaml")
	if err != nil {
		return nil, nil, err
	}
	var option map[string]any
	if err := yaml.Unmarshal(common, &option); err != nil {
		return nil, nil, fmt.Errorf("OpenAPI options: %w", err)
	}
	var fileOptions []any
	for _, name := range sortedNames(protos) {
		fileOptions = append(fileOptions, map[string]any{"file": strings.TrimPrefix(name, "api/proto/"), "option": option})
	}
	routes, err := yaml.Marshal(map[string]any{
		"type": "google.api.Service", "config_version": 3, "http": map[string]any{"rules": rules},
	})
	if err != nil {
		return nil, nil, err
	}
	options := map[string]any{"file": fileOptions}
	if len(methods) > 0 {
		options["method"] = methods
	}
	encoded, err := yaml.Marshal(map[string]any{"openapiOptions": options})
	return routes, encoded, err
}

func fragments(ctx context.Context, root *os.Root, dir string) ([]any, error) {
	inputs, err := files(ctx, root, dir, ".yaml")
	if err != nil {
		return nil, err
	}
	result := make([]any, 0)
	for _, name := range sortedNames(inputs) {
		var fragment []any
		if err := yaml.Unmarshal(inputs[name], &fragment); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		result = append(result, fragment...)
	}
	return result, nil
}

func sortedNames[V any](files map[string]V) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

type plugin struct {
	Local    []string `json:"local"`
	Out      string   `json:"out"`
	Strategy string   `json:"strategy"`
	Opt      []string `json:"opt"`
}

func pluginConfig(goTool, module, dir string) any {
	routes := "grpc_api_configuration=" + filepath.ToSlash(filepath.Join(dir, "routes.yaml"))
	options := "openapi_configuration=" + filepath.ToSlash(filepath.Join(dir, "openapi.yaml"))
	var plugins []plugin
	for _, tool := range []struct {
		name, version string
		options       []string
	}{
		{"google.golang.org/protobuf/cmd/protoc-gen-go", "v1.36.12", []string{"module=" + module}},
		{"google.golang.org/grpc/cmd/protoc-gen-go-grpc", "v1.6.0", []string{"module=" + module}},
		{"github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway", "v2.29.0", []string{"module=" + module, routes}},
		{"github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2", "v2.29.0", []string{routes, options, "disable_default_responses=true", "allow_merge=true", "merge_file_name=openapi", "openapi_naming_strategy=fqn", "disable_default_errors=true"}},
	} {
		out := dir
		if strings.HasSuffix(tool.name, "protoc-gen-openapiv2") {
			out = filepath.Join(dir, "api/http")
		}
		plugins = append(plugins, plugin{Local: []string{goTool, "run", tool.name + "@" + tool.version}, Out: out, Strategy: "all", Opt: tool.options})
	}
	return struct {
		Version string   `json:"version"`
		Plugins []plugin `json:"plugins"`
	}{"v2", plugins}
}
