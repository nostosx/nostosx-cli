package scaffold

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"

	"github.com/nostosx/nostosx-cli/internal/projectfs"
)

var infrastructurePaths = map[string]string{
	"mysql":    "infrastructure/database/mysql/mysql.go",
	"postgres": "infrastructure/database/postgres/postgres.go",
	"kafka":    "infrastructure/messaging/kafka/kafka.go",
	"nats":     "infrastructure/messaging/nats/nats.go",
}

var infrastructureDependencies = map[string]map[string]string{
	"mysql": {
		"github.com/nostosx/servex-contrib/database/sql": "v1.0.0",
		"github.com/go-sql-driver/mysql":                 "v1.10.1",
	},
	"postgres": {
		"github.com/nostosx/servex-contrib/database/sql": "v1.0.0",
		"github.com/jackc/pgx/v5":                        "v5.11.0",
		// pgx's authentication path reaches precis; require the patched text module.
		"golang.org/x/text": "v0.42.0",
	},
	"kafka": {
		"github.com/nostosx/servex-contrib/broker/kafka": "v1.2.0",
		"github.com/twmb/franz-go":                       "v1.22.2",
	},
	"nats": {
		"github.com/nostosx/servex-contrib/broker/nats": "v1.2.0",
		"github.com/nats-io/nats.go":                    "v1.54.0",
	},
}

func (o Options) MySQL() bool    { return slices.Contains(o.Infrastructure, "mysql") }
func (o Options) Postgres() bool { return slices.Contains(o.Infrastructure, "postgres") }
func (o Options) Kafka() bool    { return slices.Contains(o.Infrastructure, "kafka") }
func (o Options) NATS() bool     { return slices.Contains(o.Infrastructure, "nats") }

func normalizeInfrastructure(features []string) ([]string, error) {
	result := make([]string, 0, len(features))
	for _, feature := range features {
		if feature == "postgresql" {
			feature = "postgres"
		}
		if _, ok := infrastructurePaths[feature]; !ok {
			return nil, fmt.Errorf("unsupported infrastructure %q (use mysql, postgres, kafka or nats)", feature)
		}
		if slices.Contains(result, feature) {
			return nil, fmt.Errorf("duplicate infrastructure %q", feature)
		}
		result = append(result, feature)
	}
	slices.Sort(result)
	return result, nil
}

func projectInfrastructure(root *os.Root) ([]string, error) {
	var result []string
	for _, feature := range []string{"kafka", "mysql", "nats", "postgres"} {
		name := infrastructurePaths[feature]
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("unsafe infrastructure source %s", name)
		}
		if err := realParents(root, name); err != nil {
			return nil, err
		}
		result = append(result, feature)
	}
	if len(result) != 0 {
		content, _, err := readAPIFile(root, "infrastructure/config/config.go")
		if err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(token.NewFileSet(), "config.go", content, 0)
		if err != nil {
			return nil, err
		}
		hasValues := false
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || spec.Name.Name != "Config" {
				return true
			}
			if fields, ok := spec.Type.(*ast.StructType); ok {
				for _, field := range fields.Fields.List {
					for _, name := range field.Names {
						hasValues = hasValues || name.Name == "Values"
					}
				}
			}
			return false
		})
		if !hasValues {
			return nil, errors.New("infrastructure requires Config.Values (servex/config.Reader): add the field and assign loaded.Snapshot() in Load before adding a service; existing configuration was preserved")
		}
	}
	return result, nil
}

func addInfrastructureFiles(files map[string][]byte, o Options) error {
	for _, feature := range o.Infrastructure {
		rendered, err := render("infrastructure/"+feature, data{Options: o})
		if err != nil {
			return err
		}
		for name, content := range rendered {
			if _, exists := files[name]; exists {
				return fmt.Errorf("duplicate infrastructure output %s", name)
			}
			files[name] = content
		}
		fragment := strings.TrimSuffix(infrastructurePaths[feature], feature+".go") + "config.example.yaml"
		if _, exists := files["config.yaml"]; exists {
			files["config.yaml"] = append(files["config.yaml"], files[fragment]...)
		}
	}
	if len(o.Infrastructure) != 0 {
		updated, err := infrastructureModule(files["go.mod"], o.Infrastructure)
		if err != nil {
			return err
		}
		files["go.mod"] = updated
	}
	return nil
}

func infrastructureModule(original []byte, features []string) ([]byte, error) {
	f, err := modfile.Parse("go.mod", original, nil)
	if err != nil {
		return nil, err
	}
	if f.Module == nil {
		return nil, errors.New("go.mod must declare a module")
	}
	for _, feature := range features {
		for module, version := range infrastructureDependencies[feature] {
			for _, existing := range f.Require {
				if existing.Mod.Path == module && semver.Compare(existing.Mod.Version, version) > 0 {
					version = existing.Mod.Version
				}
			}
			if err := f.AddRequire(module, version); err != nil {
				return nil, err
			}
		}
	}
	f.Cleanup()
	f.SortBlocks()
	return f.Format()
}

// AddInfrastructure writes one backend and a configuration example, then updates
// only its go.mod requirements. Configuration, go.sum and assembly remain owned
// by the application. Generation is offline; run go mod tidy explicitly afterwards.
func AddInfrastructure(ctx context.Context, dir, feature string) (err error) {
	features, err := normalizeInfrastructure([]string{feature})
	if err != nil {
		return err
	}
	feature = features[0]
	root, release, err := projectfs.Lock(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	module, err := readModulePath(root)
	if err != nil {
		return err
	}
	original, mode, err := readAPIFile(root, "go.mod")
	if err != nil {
		return err
	}
	updated, err := infrastructureModule(original, features)
	if err != nil {
		return err
	}
	opts := Options{Module: module}
	files, err := render("infrastructure/"+feature, data{Options: opts})
	if err != nil {
		return err
	}
	return publishInfrastructure(ctx, root, files, original, updated, mode)
}

// Publish the manifest last, and roll back only new files/directories on normal
// failures. Existing files are never truncated. The shared project lock serializes
// CLI writers; a final comparison also detects external go.mod edits.
func publishInfrastructure(ctx context.Context, root *os.Root, files map[string][]byte, original, updated []byte, mode fs.FileMode) (err error) {
	var missingDirs []string
	for name := range files {
		if _, statErr := root.Lstat(name); !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("infrastructure output already exists or is inaccessible: %s", name)
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			info, statErr := root.Lstat(parent)
			switch {
			case errors.Is(statErr, fs.ErrNotExist):
				if !slices.Contains(missingDirs, parent) {
					missingDirs = append(missingDirs, parent)
				}
			case statErr != nil:
				return statErr
			case !info.IsDir() || info.Mode()&os.ModeSymlink != 0:
				return fmt.Errorf("unsafe infrastructure directory %s", parent)
			}
		}
	}
	staged := ".nostosx-infrastructure-" + rand.Text() + ".tmp"
	file, err := root.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer func() {
		if cleanup := root.Remove(staged); !errors.Is(cleanup, fs.ErrNotExist) {
			err = errors.Join(err, cleanup)
		}
	}()
	_, writeErr := file.Write(updated)
	if err := errors.Join(writeErr, file.Chmod(mode.Perm()), file.Close()); err != nil {
		return err
	}
	if err := writeFiles(ctx, root, files); err != nil {
		return err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		for name := range files {
			err = errors.Join(err, root.Remove(name))
		}
		slices.SortFunc(missingDirs, func(a, b string) int { return len(b) - len(a) })
		for _, name := range missingDirs {
			err = errors.Join(err, root.Remove(name))
		}
	}()
	current, _, err := readAPIFile(root, "go.mod")
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return errors.New("go.mod changed during generation; retry after stopping other writers")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.Rename(staged, "go.mod"); err != nil {
		return err
	}
	committed = true
	return nil
}
