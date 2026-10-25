//go:build integration

package scaffold_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

func TestContractGenerationSafety(t *testing.T) {
	binary, _ := buildCLI(t)

	o := options(t)
	o.Output = filepath.Join(t.TempDir(), "project with spaces")
	if _, err := scaffold.New(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"api"}, append(args, "--dir", o.Output)...)...)
		cmd.Dir = o.Output
		cmd.Env = append(os.Environ(), "GOWORK=off")
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	assertUnlocked := func() {
		t.Helper()
		if _, err := os.Lstat(filepath.Join(o.Output, ".nostosx.lock")); !os.IsNotExist(err) {
			t.Fatalf("generator left a lock: %v", err)
		}
	}

	t.Run("empty-project-needs-no-external-tools", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		before := snapshot(t, o.Output)
		for _, args := range [][]string{{"lint"}, {"check"}, {"generate"}} {
			if output, err := invoke(args...); err != nil {
				t.Fatalf("empty generation %v: %v\n%s", args, err, output)
			}
		}
		if !reflect.DeepEqual(before, snapshot(t, o.Output)) {
			t.Fatal("empty generation modified the project")
		}
	})

	for _, kind := range []string{"context", "service"} {
		if err := scaffold.Add(t.Context(), o.Output, kind, "order"); err != nil {
			t.Fatal(err)
		}
	}
	if err := scaffold.AddAPI(t.Context(), o.Output, scaffold.APIOptions{Name: "get-order", Service: "order", Method: "GET", Path: "/v1/orders/{order_id}"}); err != nil {
		t.Fatal(err)
	}
	openapi := filepath.Join(o.Output, "api/http/openapi.yaml")
	write(t, openapi, strings.ReplaceAll(read(t, openapi), "title:", "title: 应用 #"))
	if output, err := invoke("generate"); err != nil {
		t.Fatalf("generate: %v\n%s", err, output)
	}
	assertUnlocked()
	outputs := func() map[string]string {
		all := snapshot(t, o.Output)
		result := make(map[string]string)
		for name, data := range all {
			if strings.HasPrefix(name, "api/gen/go/") || name == "api/http/openapi.swagger.json" {
				result[name] = data
			}
		}
		return result
	}
	published := outputs()
	if len(published) == 0 {
		t.Fatal("generator produced no output")
	}
	baseline := t.TempDir()
	if err := os.CopyFS(baseline, os.DirFS(o.Output)); err != nil {
		t.Fatal(err)
	}
	if output, err := invoke("breaking", "--against", baseline); err != nil {
		t.Fatalf("unchanged contract rejected: %v\n%s", err, output)
	}

	t.Run("portable-output", func(t *testing.T) {
		for name, data := range published {
			if bytes.ContainsRune([]byte(data), '\r') || !strings.HasSuffix(data, "\n") {
				t.Fatalf("non-LF output: %s", name)
			}
		}
		var document struct {
			Info struct{ Title string }
		}
		if err := json.Unmarshal([]byte(published["api/http/openapi.swagger.json"]), &document); err != nil {
			t.Fatal(err)
		}
		if document.Info.Title != "应用" {
			t.Fatalf("title=%q", document.Info.Title)
		}
	})

	t.Run("existing-lock-preserves-owner", func(t *testing.T) {
		lock := filepath.Join(o.Output, ".nostosx.lock")
		write(t, lock, "another invocation")
		defer os.Remove(lock)
		if output, err := invoke("generate"); err == nil || !strings.Contains(output, "another generator") {
			t.Fatalf("lock accepted: %v\n%s", err, output)
		}
		if read(t, lock) != "another invocation" || !reflect.DeepEqual(published, outputs()) {
			t.Fatal("generator modified the owner's lock or published output")
		}
	})

	t.Run("failed-plugin-keeps-published-output", func(t *testing.T) {
		// The Go plugin rejects a valid Proto with no Go import path.
		bad := filepath.Join(o.Output, "api/proto/broken.proto")
		write(t, bad, "syntax = \"proto3\";\npackage broken;\nmessage Broken {}\n")
		defer os.Remove(bad)
		if output, err := invoke("generate"); err == nil || !strings.Contains(output, "unable to determine Go import path") {
			t.Fatalf("expected plugin failure: %v\n%s", err, output)
		}
		assertUnlocked()
		if !reflect.DeepEqual(published, outputs()) {
			t.Fatal("failed generation changed published output")
		}
	})

	t.Run("deleted-contract-cleans-stale-output", func(t *testing.T) {
		if err := os.RemoveAll(filepath.Join(o.Output, "api/proto")); err != nil {
			t.Fatal(err)
		}
		if output, err := invoke("breaking", "--against", baseline); err == nil {
			t.Fatalf("deleted contracts skipped compatibility check: %s", output)
		}
		if output, err := invoke("check"); err == nil || !strings.Contains(output, "generated contracts differ") {
			t.Fatalf("stale output accepted: %v\n%s", err, output)
		}
		assertUnlocked()
		if output, err := invoke("generate"); err != nil {
			t.Fatalf("clean stale output: %v\n%s", err, output)
		}
		assertUnlocked()
		if len(outputs()) != 0 {
			t.Fatal("stale outputs remain after removing contracts")
		}
	})
}
