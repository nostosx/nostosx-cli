package scaffold

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var literalSegment = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
var parameterSegment = regexp.MustCompile(`^\{([a-z][a-z0-9]*(?:_[a-z0-9]+)*)\}$`)

func routeSegments(route string) ([]string, error) {
	if len(route) > 2048 || !strings.HasPrefix(route, "/") {
		return nil, errors.New("HTTP path must be absolute and at most 2048 bytes")
	}
	if route == "/" {
		return nil, nil
	}
	parts := strings.Split(route[1:], "/")
	params := make(map[string]bool)
	for _, part := range parts {
		if parameterSegment.MatchString(part) {
			if params[part] {
				return nil, errors.New("HTTP path parameters must be unique")
			}
			params[part] = true
			continue
		}
		if part == "." || part == ".." || !literalSegment.MatchString(part) {
			return nil, errors.New("HTTP path requires literal segments or simple {field_name} parameters; no queries, wildcards or trailing slash")
		}
	}
	return parts, nil
}

func routesOverlap(a, b string) bool {
	x, _ := routeSegments(a)
	y, _ := routeSegments(b)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] && !strings.HasPrefix(x[i], "{") && !strings.HasPrefix(y[i], "{") {
			return false
		}
	}
	return true
}

type httpBinding struct {
	Selector     string                       `yaml:"selector"`
	Get          string                       `yaml:"get"`
	Post         string                       `yaml:"post"`
	Put          string                       `yaml:"put"`
	Patch        string                       `yaml:"patch"`
	Delete       string                       `yaml:"delete"`
	Body         string                       `yaml:"body"`
	ResponseBody string                       `yaml:"response_body"`
	Custom       *struct{ Kind, Path string } `yaml:"custom"`
	Additional   []httpBinding                `yaml:"additional_bindings"`
}

func checkRoutes(ctx context.Context, root *os.Root, o APIOptions) error {
	selector := o.Service + ".v1." + strings.ToUpper(o.Service[:1]) + o.Service[1:] + "Service." + o.rpcName()
	var check func(httpBinding, int) error
	check = func(binding httpBinding, depth int) error {
		if depth > 8 {
			return errors.New("HTTP additional_bindings nesting exceeds 8")
		}
		if binding.Selector == selector || strings.ContainsAny(binding.Selector, "*,") {
			return fmt.Errorf("HTTP selector %q conflicts with %s or is not exact", binding.Selector, selector)
		}
		if binding.Custom != nil {
			return errors.New("add api cannot check custom HTTP bindings; use standard method bindings")
		}
		count := 0
		for method, route := range map[string]string{"GET": binding.Get, "POST": binding.Post, "PUT": binding.Put, "PATCH": binding.Patch, "DELETE": binding.Delete} {
			if route == "" {
				continue
			}
			count++
			if _, err := routeSegments(route); err != nil {
				return fmt.Errorf("cannot check existing route %q: %w", route, err)
			}
			if method == o.Method && routesOverlap(route, o.Path) {
				return fmt.Errorf("HTTP %s %s overlaps existing route %s", method, o.Path, route)
			}
		}
		if count != 1 {
			return errors.New("each HTTP binding must contain exactly one standard method")
		}
		for _, extra := range binding.Additional {
			if err := check(extra, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return scanAPIFiles(ctx, root, "api/http/routes", ".yaml", func(name string, b []byte) error {
		decoder := yaml.NewDecoder(strings.NewReader(string(b)))
		decoder.KnownFields(true)
		var bindings []httpBinding
		if err := decoder.Decode(&bindings); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("parse HTTP bindings %s: %w", name, err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return fmt.Errorf("HTTP bindings %s must contain a single YAML document", name)
		}
		for _, binding := range bindings {
			if err := check(binding, 0); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil
	})
}
