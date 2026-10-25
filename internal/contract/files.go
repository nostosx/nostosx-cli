package contract

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

var targets = []string{"api/gen/go", "api/http/openapi.swagger.json"}

// Reject symlinks as well as escapes: deleting an in-root alias could otherwise
// destroy a different directory inside the same project.
func realParents(root *os.Root, name string) error {
	parent := path.Dir(name)
	if parent == "." {
		return nil
	}
	if err := realParents(root, parent); err != nil {
		return err
	}
	info, err := root.Lstat(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe contract directory %s", parent)
	}
	return nil
}

func readFile(root *os.Root, name string) ([]byte, error) {
	if err := realParents(root, name); err != nil {
		return nil, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	const limit = 16 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("%s must be a regular file of at most 16 MiB", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(data) > limit {
		return nil, fmt.Errorf("%s exceeds 16 MiB", name)
	}
	return data, err
}

func files(ctx context.Context, root *os.Root, dir, suffix string) (map[string][]byte, error) {
	if err := realParents(root, dir); err != nil {
		return nil, err
	}
	result := make(map[string][]byte)
	count, size := 0, 0
	err := fs.WalkDir(root.FS(), dir, func(name string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && name == dir {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > 4096 || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("contract tree exceeds 4096 entries or contains a symlink: %s", name)
		}
		if entry.IsDir() || !strings.HasSuffix(name, suffix) {
			return nil
		}
		data, err := readFile(root, name)
		if err != nil {
			return err
		}
		size += len(data)
		if size > 64<<20 {
			return errors.New("contract tree exceeds 64 MiB")
		}
		result[name] = data
		return nil
	})
	return result, err
}

func outputs(ctx context.Context, root *os.Root) (map[string][]byte, error) {
	result, err := files(ctx, root, targets[0], "")
	if err != nil {
		return nil, err
	}
	data, err := readFile(root, targets[1])
	if err == nil {
		result[targets[1]] = data
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return result, nil
}

func equalFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, data := range a {
		other, ok := b[name]
		if !ok || !bytes.Equal(data, other) {
			return false
		}
	}
	return true
}

// Stage both artifacts before replacing either. Retain backups until both
// renames succeed; roll back normal I/O failures. This is not a crash transaction.
func publish(root *os.Root, want map[string][]byte) (err error) {
	stage := ".nostosx-api-" + rand.Text()
	if err := root.Mkdir(stage, 0o700); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			err = errors.Join(err, root.RemoveAll(stage))
		}
	}()
	for _, name := range sortedNames(want) {
		if err := root.MkdirAll(path.Join(stage, "new", path.Dir(name)), 0o755); err != nil {
			return err
		}
		f, err := root.OpenFile(path.Join(stage, "new", name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(want[name])
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
	}
	type replacement struct {
		name           string
		backed, placed bool
	}
	var changes []replacement
	defer func() {
		if err == nil {
			return
		}
		var rollbackErr error
		for _, change := range slices.Backward(changes) {
			if change.placed {
				rollbackErr = errors.Join(rollbackErr, root.RemoveAll(change.name))
			}
			if change.backed {
				rollbackErr = errors.Join(rollbackErr, root.Rename(path.Join(stage, "old", change.name), change.name))
			}
		}
		if rollbackErr != nil {
			// Preserve recoverable backups if rollback itself failed.
			err = errors.Join(err, fmt.Errorf("rollback failed; inspect %s: %w", stage, rollbackErr))
			cleanup = false
		}
	}()
	for _, name := range targets {
		if err := realParents(root, name); err != nil {
			return err
		}
		if err := root.MkdirAll(path.Dir(name), 0o755); err != nil {
			return err
		}
		changes = append(changes, replacement{name: name})
		change := &changes[len(changes)-1]
		if _, err := root.Lstat(name); err == nil {
			if err := root.MkdirAll(path.Join(stage, "old", path.Dir(name)), 0o700); err != nil {
				return err
			}
			if err := root.Rename(name, path.Join(stage, "old", name)); err != nil {
				return err
			}
			change.backed = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		newName := path.Join(stage, "new", name)
		if _, err := root.Lstat(newName); err == nil {
			if err := root.Rename(newName, name); err != nil {
				return err
			}
			change.placed = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
