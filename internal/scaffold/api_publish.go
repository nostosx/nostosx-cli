package scaffold

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
)

type apiUpdate struct {
	name          string
	before, after []byte
	mode          fs.FileMode
}

// Publish staged replacements under the project lock, restoring original bytes
// and permissions on ordinary errors. The contract is deliberately replaced last.
func publishAPI(ctx context.Context, root *os.Root, files map[string][]byte, updates []apiUpdate) (err error) {
	for name := range files {
		if err := realParents(root, name); err != nil {
			return err
		}
		if _, err := root.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("API output already exists or is inaccessible: %s", name)
		}
	}
	staged := make([]string, len(updates))
	defer func() {
		for _, name := range staged {
			if name != "" {
				if e := root.Remove(name); !errors.Is(e, fs.ErrNotExist) {
					err = errors.Join(err, e)
				}
			}
		}
	}()
	for i, u := range updates {
		current, _, e := readAPIFile(root, u.name)
		if e != nil {
			return e
		}
		if !bytes.Equal(current, u.before) {
			return fmt.Errorf("%s changed during generation", u.name)
		}
		staged[i] = path.Join(path.Dir(u.name), ".nostosx-api-"+rand.Text()+".tmp")
		if e := writeReplacement(root, staged[i], u.after, u.mode); e != nil {
			return e
		}
	}
	if err := writeFiles(ctx, root, files); err != nil {
		return err
	}
	replaced := 0
	committed := false
	defer func() {
		if committed {
			return
		}
		for i := replaced - 1; i >= 0; i-- {
			u := updates[i]
			tmp := path.Join(path.Dir(u.name), ".nostosx-rollback-"+rand.Text()+".tmp")
			e := writeReplacement(root, tmp, u.before, u.mode)
			if e == nil {
				e = root.Rename(tmp, u.name)
			}
			err = errors.Join(err, e)
			if e != nil {
				_ = root.Remove(tmp)
			}
		}
		for name := range files {
			err = errors.Join(err, root.Remove(name))
		}
	}()
	for i, u := range updates {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, _, e := readAPIFile(root, u.name)
		if e != nil {
			return e
		}
		if !bytes.Equal(current, u.before) {
			return fmt.Errorf("%s changed during generation", u.name)
		}
		if err := root.Rename(staged[i], u.name); err != nil {
			return err
		}
		replaced++
	}
	committed = true
	return nil
}
func writeReplacement(root *os.Root, name string, b []byte, mode fs.FileMode) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	return errors.Join(writeErr, f.Chmod(mode.Perm()), f.Close())
}
