package scaffold

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/nostosx/nostosx-cli/internal/projectfs"
)

// APIOptions describes a native HTTP operation, RPC or gateway binding.
type APIOptions struct {
	Name, Service, Context, Method, Path string
	Transport, Kind                      string
}

// Validate checks explicit API inputs without reading or writing a project.
func (o APIOptions) Validate() error {
	if err := validateName(o.Name, false); err != nil {
		return fmt.Errorf("API: %w", err)
	}
	if err := validateName(o.Service, true); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	if err := validateName(o.Context, true); err != nil {
		return fmt.Errorf("context (required): %w", err)
	}
	if o.Transport != "" && o.Transport != "http" && o.Transport != "grpc" && o.Transport != "gateway" {
		return errors.New("API transport must be http, grpc or gateway")
	}
	if o.Kind != "" && o.Kind != "command" && o.Kind != "query" {
		return errors.New("kind must be command or query")
	}
	if o.Transport == "grpc" {
		if o.Kind == "" {
			return errors.New("gRPC-only APIs require --kind command or query")
		}
		if o.Method != "" || o.Path != "" {
			return errors.New("gRPC-only APIs do not accept --method or --path; use --transport gateway for HTTP mapping")
		}
		return nil
	}
	if o.Method == "" || o.Path == "" {
		return errors.New("--method and --path must be provided together")
	}
	switch o.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return errors.New("method must be GET, POST, PUT, PATCH or DELETE")
	}
	if _, err := routeSegments(o.Path); err != nil {
		return err
	}
	for _, reserved := range []string{"/livez", "/readyz"} {
		if routesOverlap(o.Path, reserved) {
			return fmt.Errorf("path overlaps reserved health route %s", reserved)
		}
	}
	return nil
}

func (o APIOptions) rpcName() string {
	var out strings.Builder
	for part := range strings.SplitSeq(o.Name, "-") {
		out.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return out.String()
}

// AddAPI adds a typed CQRS use case, transport adapter and service wiring.
// It runs offline under the shared generator lock. Business logic and authorization
// remain application-owned. Cooperative writers must
// honor the lock; rollback covers normal errors, not process termination.
func AddAPI(ctx context.Context, dir string, o APIOptions) (err error) {
	if err := o.Validate(); err != nil {
		return err
	}
	root, release, err := projectfs.Lock(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	module, err := readModulePath(root)
	if err != nil {
		return err
	}
	transport, err := projectTransport(root)
	if err != nil {
		return err
	}
	if o.Transport == "" || o.Transport == "http" {
		if transport == "grpc" {
			return errors.New("native HTTP API requires an HTTP-enabled project")
		}
		return addHTTPAPI(ctx, root, module, o)
	}
	if transport == "http" {
		return errors.New("proto API requires a gRPC-enabled project (new --transport grpc or http,grpc)")
	}
	if o.Transport == "gateway" && transport != "http,grpc" {
		return errors.New("gateway API requires both HTTP and gRPC transports")
	}
	protoPath := "api/proto/" + o.Service + "/v1/" + o.Service + ".proto"
	original, mode, err := readAPIFile(root, protoPath)
	if err != nil {
		return fmt.Errorf("read existing service: %w", err)
	}
	updated, err := extendContract(ctx, root, protoPath, original, module, o)
	if err != nil {
		return err
	}
	if err := checkAdapter(ctx, root, o); err != nil {
		return err
	}
	if o.Method != "" {
		if err := checkRoutes(ctx, root, o); err != nil {
			return err
		}
		if err := checkHTTPRoutes(ctx, root, o); err != nil {
			return err
		}
	}
	files, err := apiFiles(module, o)
	if err != nil {
		return err
	}
	return addCQRSAPI(ctx, root, module, o, files, protoPath, original, updated, mode)

}

func realParents(root *os.Root, name string) error {
	parent := path.Dir(name)
	if parent != "." {
		if err := realParents(root, parent); err != nil {
			return err
		}
		info, err := root.Lstat(parent)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe API directory %s", parent)
		}
	}
	return nil
}

func readAPIFile(root *os.Root, name string) ([]byte, fs.FileMode, error) {
	if err := realParents(root, name); err != nil {
		return nil, 0, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, 0, err
	}
	const limit = 1 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, 0, fmt.Errorf("%s must be a regular file of at most 1 MiB", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > limit {
		return nil, 0, fmt.Errorf("%s exceeds 1 MiB", name)
	}
	return b, info.Mode(), err
}

// scanAPIFiles bounds parser work and rejects symlinks instead of following them.
func scanAPIFiles(ctx context.Context, root *os.Root, dir, suffix string, visit func(string, []byte) error) error {
	if err := realParents(root, dir+"/entry"); err != nil {
		return err
	}
	count, size := 0, 0
	return fs.WalkDir(root.FS(), dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > 512 || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("API input tree exceeds 512 entries or contains a symlink: %s", name)
		}
		if entry.IsDir() || !strings.HasSuffix(name, suffix) {
			return nil
		}
		b, _, err := readAPIFile(root, name)
		if err != nil {
			return err
		}
		size += len(b)
		if size > 16<<20 {
			return errors.New("API input tree exceeds 16 MiB")
		}
		return visit(name, b)
	})
}

type apiData struct {
	File, RPC, Method, Path          string
	Kind, KindType, Field, Operation string
	Body                             bool
	Fields                           []apiField
}

type apiField struct {
	Name   string
	GoName string
	Number int
}

func apiFiles(module string, o APIOptions) (map[string][]byte, error) {
	api := cqrsData(o)
	api.Method = strings.ToLower(o.Method)
	api.Body = o.Method == http.MethodPost || o.Method == http.MethodPut || o.Method == http.MethodPatch
	files, err := render("api", data{Module: module, Service: o.Service, Context: o.Context, ServiceType: strings.ToUpper(o.Service[:1]) + o.Service[1:], API: api})
	if o.Method == "" {
		delete(files, "api/http/routes/"+o.Service+"_"+api.File+".yaml")
	}
	return files, err
}
