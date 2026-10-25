// Package projectfs coordinates writers to a generated project.
package projectfs

import (
	"errors"
	"fmt"
	"os"
)

// Lock opens a real project directory and exclusively creates its shared lock.
// Callers must release it after closing all project files. A lock left by a
// terminated process requires the operator to confirm that its owner has stopped.
func Lock(dir string) (*os.Root, func() error, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("project directory must be a real directory")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	f, err := root.OpenFile(".nostosx.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("lock project (another generator may be running): %w", err), root.Close())
	}
	release := func() error { return errors.Join(root.Remove(".nostosx.lock"), root.Close()) }
	if err := f.Close(); err != nil {
		return nil, nil, errors.Join(err, release())
	}
	return root, release, nil
}
