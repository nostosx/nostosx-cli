package scaffold_test

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func TestInfrastructureSelections(t *testing.T) {
	t.Parallel()
	features := []string{"mysql", "postgres", "kafka", "nats"}
	paths := []string{"database/mysql/mysql.go", "database/postgres/postgres.go", "messaging/kafka/kafka.go", "messaging/nats/nats.go"}
	for mask := range 16 {
		t.Run(fmt.Sprintf("selection-%04b", mask), func(t *testing.T) {
			t.Parallel()
			o := options(t)
			for i, feature := range features {
				if mask&(1<<i) != 0 {
					o.Infrastructure = append(o.Infrastructure, feature)
				}
			}
			if _, err := scaffold.New(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			files := snapshot(t, o.Output)
			for i, feature := range features {
				selected := mask&(1<<i) != 0
				_, exists := files["infrastructure/"+paths[i]]
				if exists != selected {
					t.Fatalf("%s selected=%v exists=%v", feature, selected, exists)
				}
				if strings.Contains(files["config.yaml"], feature+":\n") != selected {
					t.Fatal("configuration selection mismatch", feature)
				}
				if strings.Contains(files["app.go"], feature+"infra.New(") != selected {
					t.Fatal("assembly selection mismatch", feature)
				}
			}
			for name, content := range files {
				if strings.HasSuffix(name, ".go") {
					if _, err := parser.ParseFile(token.NewFileSet(), name, content, parser.AllErrors); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := modfile.Parse("go.mod", []byte(files["go.mod"]), nil); err != nil {
				t.Fatal(err)
			}
			if mask&2 != 0 && !strings.Contains(files["go.mod"], "golang.org/x/text v0.42.0") {
				t.Fatal("PostgreSQL must pin the patched text dependency floor")
			}
			o.Output = filepath.Join(t.TempDir(), "second")
			if _, err := scaffold.New(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(files, snapshot(t, o.Output)) {
				t.Fatal("infrastructure generation is not reproducible")
			}
		})
	}
}

func TestAddInfrastructurePreservesApplication(t *testing.T) {
	t.Parallel()
	for _, feature := range []string{"mysql", "postgresql", "kafka", "nats"} {
		t.Run(feature, func(t *testing.T) {
			t.Parallel()
			o := options(t)
			if _, err := scaffold.New(t.Context(), o); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, o.Output)
			if err := scaffold.AddInfrastructure(t.Context(), o.Output, feature); err != nil {
				t.Fatal(err)
			}
			after := snapshot(t, o.Output)
			for name, content := range before {
				if name != "go.mod" && after[name] != content {
					t.Fatalf("addition rewrote %s", name)
				}
			}
			if !strings.Contains(after["go.mod"], "servex-contrib/") {
				t.Fatal("missing adapter dependency")
			}
			if err := scaffold.AddInfrastructure(t.Context(), o.Output, feature); err == nil {
				t.Fatal("duplicate accepted")
			}
			if !reflect.DeepEqual(after, snapshot(t, o.Output)) {
				t.Fatal("failed duplicate changed output")
			}
			if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err != nil {
				t.Fatal(err)
			}
			if err := scaffold.Add(t.Context(), o.Output, "service", "order"); err != nil {
				t.Fatal(err)
			}
			name := feature
			if name == "postgresql" {
				name = "postgres"
			}
			if !strings.Contains(read(t, filepath.Join(o.Output, "services/order/app.go")), name+"infra.New(") {
				t.Fatal("service did not inherit infrastructure")
			}
		})
	}
}

func TestInfrastructureRejectsUnsafeChanges(t *testing.T) {
	t.Parallel()
	for _, features := range [][]string{{"redis"}, {"mysql", "mysql"}, {"postgres", "postgresql"}} {
		o := options(t)
		o.Infrastructure = features
		if _, err := scaffold.New(t.Context(), o); err == nil {
			t.Fatal("invalid selection accepted", features)
		}
		if _, err := os.Stat(o.Output); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid selection created output")
		}
	}
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, o.Output)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := scaffold.AddInfrastructure(ctx, o.Output, "mysql"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("canceled addition changed project")
	}
	if err := scaffold.AddInfrastructure(t.Context(), o.Output, "../mysql"); err == nil {
		t.Fatal("traversal accepted")
	}
	if err := os.MkdirAll(filepath.Join(o.Output, "infrastructure/database/mysql"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Output, "infrastructure/database/mysql/config.example.yaml"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	before = snapshot(t, o.Output)
	if err := scaffold.AddInfrastructure(t.Context(), o.Output, "mysql"); err == nil {
		t.Fatal("existing config overwritten")
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("collision changed project")
	}
}

func TestInfrastructureKeepsHigherVersionsAndComments(t *testing.T) {
	t.Parallel()
	o := options(t)
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(o.Output, "go.mod")
	content := read(t, p) + "\n// Application pin.\nrequire github.com/go-sql-driver/mysql v1.99.0\n"
	if err := os.WriteFile(p, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.AddInfrastructure(t.Context(), o.Output, "mysql"); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if !strings.Contains(got, "github.com/go-sql-driver/mysql v1.99.0") || !strings.Contains(got, "// Application pin.") {
		t.Fatal(got)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatal(info, err)
	}
}

func TestInfrastructureServiceRejectsLegacyConfig(t *testing.T) {
	t.Parallel()
	o := options(t)
	o.Infrastructure = []string{"mysql"}
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(o.Output, "infrastructure/config/config.go")
	legacy := strings.ReplaceAll(read(t, p), "Values", "LegacyReader")
	if err := os.WriteFile(p, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := scaffold.Add(t.Context(), o.Output, "context", "order"); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, o.Output)
	if err := scaffold.Add(t.Context(), o.Output, "service", "order"); err == nil || !strings.Contains(err.Error(), "Config.Values") {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
		t.Fatal("legacy config rejection changed project")
	}
}
