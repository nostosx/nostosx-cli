package contract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The test binary also supplies a portable Buf fixture. Real plugin generation
// is exercised separately by the scaffold integration suite.
func TestMain(m *testing.M) {
	if os.Getenv("NOSTOSX_BUF_FIXTURE") == "1" {
		if len(os.Args) == 2 && os.Args[1] == "--version" {
			fmt.Println(os.Getenv("NOSTOSX_BUF_VERSION"))
		} else {
			fmt.Println(strings.Join(os.Args[1:], "|"))
			if os.Getenv("NOSTOSX_BUF_BLOCK") == "1" {
				for {
					time.Sleep(time.Hour)
				}
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func bufFixture(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	name := "buf"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("NOSTOSX_BUF_FIXTURE", "1")
	t.Setenv("NOSTOSX_BUF_VERSION", bufVersion)
}

func TestBufBoundary(t *testing.T) {
	bufFixture(t)
	dir := t.TempDir()
	mustWrite(t, dir, "buf.yaml", "version: v2\n")
	for _, tc := range []struct{ action, against, want string }{
		{"breaking", ".git#ref=feature with spaces", "breaking|--against|.git#ref=feature with spaces"},
		{"lint", "", "lint"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			if tc.action == "lint" {
				mustWrite(t, dir, "api/proto/service.proto", "syntax = \"proto3\";")
			}
			var out strings.Builder
			if err := Run(t.Context(), dir, tc.action, tc.against, &out, io.Discard); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(out.String()) != tc.want {
				t.Fatalf("arguments changed: %q", out.String())
			}
		})
	}
	t.Setenv("NOSTOSX_BUF_VERSION", "0.0.0")
	if err := Run(t.Context(), dir, "lint", "", io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "Buf "+bufVersion) {
		t.Fatalf("accepted wrong Buf version: %v", err)
	}
	if err := Run(t.Context(), dir, "breaking", "", io.Discard, io.Discard); err == nil {
		t.Fatal("accepted implicit baseline")
	}
	assertNoLock(t, dir)
}

type cancelWriter struct{ cancel context.CancelFunc }

func (w cancelWriter) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestCanceledCommandReleasesLock(t *testing.T) {
	bufFixture(t)
	t.Setenv("NOSTOSX_BUF_BLOCK", "1")
	dir := t.TempDir()
	mustWrite(t, dir, "buf.yaml", "version: v2\n")
	mustWrite(t, dir, "api/proto/service.proto", "syntax = \"proto3\";")
	mustWrite(t, dir, "api/gen/go/old.pb.go", "old output")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err := Run(ctx, dir, "lint", "", cancelWriter{cancel}, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "api/gen/go/old.pb.go"))
	if err != nil || string(data) != "old output" {
		t.Fatalf("cancellation changed output: %s, %v", data, err)
	}
	assertNoLock(t, dir)
}

func TestSymlinkedContractsAreRejected(t *testing.T) {
	for _, name := range []string{"api/proto", "api/gen/go", "api/http/openapi.swagger.json"} {
		t.Run(name, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			mustWrite(t, outside, "keep", "untouched")
			link := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				if runtime.GOOS == "windows" {
					t.Skip("symlink privilege unavailable: ", err)
				}
				t.Fatal(err)
			}
			if err := Run(t.Context(), dir, "generate", "", io.Discard, io.Discard); err == nil {
				t.Fatal("accepted symlinked contract path")
			}
			data, err := os.ReadFile(filepath.Join(outside, "keep"))
			if err != nil || string(data) != "untouched" {
				t.Fatal("changed symlink target")
			}
			assertNoLock(t, dir)
		})
	}
}

func TestPublicationRollsBackFirstArtifact(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "api/gen/go/old.pb.go", "old output")
	// The second artifact has an invalid parent. The first replacement must be
	// restored even when publication fails after its rename has succeeded.
	mustWrite(t, dir, "api/http", "not a directory")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := publish(root, map[string][]byte{"api/gen/go/new.pb.go": []byte("new output")}); err == nil {
		t.Fatal("accepted invalid output parent")
	}
	data, err := root.ReadFile("api/gen/go/old.pb.go")
	if err != nil || string(data) != "old output" {
		t.Fatalf("failed to restore old output: %s, %v", data, err)
	}
	if _, err := root.Stat("api/gen/go/new.pb.go"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new output survived rollback")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".nostosx-api-") {
			t.Fatal("successful rollback left staging files")
		}
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	name = filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertNoLock(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(dir, ".nostosx.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock left behind: %v", err)
	}
}
