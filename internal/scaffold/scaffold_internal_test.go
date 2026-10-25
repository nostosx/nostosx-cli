package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFilesKeepsOpenedProjectRoot(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot rename a directory while its root handle is open")
	}
	dir := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	moved := dir + "-moved"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeFiles(t.Context(), root, map[string][]byte{"domain/order/status.go": []byte("owned output")}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(moved, "domain", "order", "status.go"))
	if err != nil || string(got) != "owned output" {
		t.Fatalf("opened root output=%q error=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "domain")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("replacement directory was changed: %v", err)
	}
}
