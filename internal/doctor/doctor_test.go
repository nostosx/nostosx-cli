package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func TestInspectProjectAndTools(t *testing.T) {
	t.Parallel()
	for _, contracts := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "contracts"}[contracts], func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "project")
			if _, err := scaffold.New(t.Context(), scaffold.Options{Name: "demo", Module: "example.com/demo", Layout: "monolith", Transport: "http,grpc", Output: dir}); err != nil {
				t.Fatal(err)
			}
			if contracts {
				for _, kind := range []string{"context", "service"} {
					if err := scaffold.Add(t.Context(), dir, kind, "order"); err != nil {
						t.Fatal(err)
					}
				}
			}
			before := fileContents(t, dir)
			var calls []string
			report := inspect(t.Context(), dir, "test", func(_ context.Context, tool string, args ...string) (string, error) {
				calls = append(calls, tool+" "+strings.Join(args, " "))
				if tool == "go" {
					return "go version go1.27.3 darwin/arm64\n", nil
				}
				return "1.72.0\n", nil
			})
			if !report.Healthy() {
				t.Fatalf("%+v", report)
			}
			want := []string{"go version"}
			if contracts {
				want = append(want, "buf --version")
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("commands=%v", calls)
			}
			if !reflect.DeepEqual(before, fileContents(t, dir)) {
				t.Fatal("doctor changed project")
			}
			b, err := json.Marshal(report)
			if err != nil || !strings.Contains(string(b), `"version":"test"`) {
				t.Fatalf("JSON %s: %v", b, err)
			}
		})
	}
}

func TestInspectFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, toolOutput, file, content string
		toolError                       bool
	}{
		{name: "old Go", toolOutput: "go version go1.26.1 linux/amd64"},
		{name: "missing tool", toolError: true},
		{name: "bad module", toolOutput: "go version go1.27.3 linux/amd64", file: "go.mod", content: "module example.com/demo\ngo 1.24.0\n"},
		{name: "newer project", toolOutput: "go version go1.27.3 linux/amd64", file: "go.mod", content: "module example.com/demo\ngo 1.28.0\n"},
		{name: "bad config", toolOutput: "go version go1.27.3 linux/amd64", file: "buf.yaml", content: "version: v1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "project")
			if _, err := scaffold.New(t.Context(), scaffold.Options{Name: "demo", Module: "example.com/demo", Layout: "monolith", Transport: "http,grpc", Output: dir}); err != nil {
				t.Fatal(err)
			}
			if tc.file != "" {
				if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			report := inspect(t.Context(), dir, "test", func(context.Context, string, ...string) (string, error) {
				if tc.toolError {
					return "", errors.New("not installed")
				}
				return tc.toolOutput, nil
			})
			if report.Healthy() {
				t.Fatalf("accepted %+v", report)
			}
		})
	}
}

func TestInspectRejectsSymlinkAndCancellation(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "project")
	if _, err := scaffold.New(t.Context(), scaffold.Options{Name: "demo", Module: "example.com/demo", Layout: "monolith", Transport: "http,grpc", Output: dir}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	run := func(context.Context, string, ...string) (string, error) {
		return "go version go1.27.3 linux/amd64", nil
	}
	if report := inspect(ctx, dir, "test", run); report.Healthy() {
		t.Fatal("ignored cancellation")
	}
	name := filepath.Join(dir, "api/proto/link")
	if err := os.Symlink(t.TempDir(), name); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if report := inspect(t.Context(), dir, "test", run); report.Healthy() {
		t.Fatal("accepted symlink")
	}
}

func TestInspectRequiresPinnedBufForContracts(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "project")
	if _, err := scaffold.New(t.Context(), scaffold.Options{Name: "demo", Module: "example.com/demo", Layout: "monolith", Transport: "http,grpc", Output: dir}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"context", "service"} {
		if err := scaffold.Add(t.Context(), dir, kind, "order"); err != nil {
			t.Fatal(err)
		}
	}
	report := inspect(t.Context(), dir, "test", func(_ context.Context, tool string, _ ...string) (string, error) {
		if tool == "go" {
			return "go version go1.27.3 linux/amd64", nil
		}
		return "1.71.0", nil
	})
	if report.Healthy() || report.Checks[len(report.Checks)-1].Name != "buf" || report.Checks[len(report.Checks)-1].Status != "fail" {
		t.Fatalf("%+v", report)
	}
}

func TestToolVersionHonorsCancellationAndOutputLimit(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := toolVersion(ctx, "go", "version"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	var output limitedOutput
	if _, err := output.Write(make([]byte, 4097)); err == nil || output.text.Len() != 0 {
		t.Fatal("accepted excessive output")
	}
}

func fileContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		b, err := os.ReadFile(name)
		files[name] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
