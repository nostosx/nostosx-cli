// Package contract builds transport contracts with pinned external tools.
// Unlike source scaffolding, these operations execute Go plugins and Buf in a
// trusted project. They are not a sandbox for untrusted repositories or tools.
package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/nostosx/nostosx-cli/internal/projectfs"
)

// RequiredBufVersion is the supported contract tool version.
const RequiredBufVersion = "1.72.0"

const bufVersion = RequiredBufVersion

// Run performs generate, lint, check or breaking under the shared project lock.
// Against is a Buf input (for example .git#ref=main), required only for breaking.
// Check never modifies published output; generate publishes only after all
// plugins succeed. Cancellation stops external commands before publication.
func Run(ctx context.Context, dir, action, against string, stdout, stderr io.Writer) (err error) {
	if action != "generate" && action != "lint" && action != "check" && action != "breaking" {
		return fmt.Errorf("unknown contract action %q", action)
	}
	if action == "breaking" && strings.TrimSpace(against) == "" {
		return errors.New("breaking requires an explicit --against baseline")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}
	root, release, err := projectfs.Lock(dir)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	protos, err := files(ctx, root, "api/proto", ".proto")
	if err != nil {
		return err
	}
	// An empty skeleton needs no Go, Buf or placeholder contracts. A breaking
	// check still consults its baseline so deleting the last API is detected.
	if len(protos) == 0 && action == "lint" {
		return nil
	}
	r := runner{ctx: ctx, dir: dir, stdout: stdout, stderr: stderr}
	if len(protos) > 0 || action == "breaking" {
		if _, err := readFile(root, "buf.yaml"); err != nil {
			return err
		}
		var version bytes.Buffer
		if err := r.command(&version, "buf", "--version"); err != nil {
			return err
		}
		if found := strings.TrimSpace(version.String()); found != bufVersion {
			return fmt.Errorf("require Buf %s (found %s)", bufVersion, found)
		}
	}
	switch action {
	case "lint":
		return r.command(stdout, "buf", "lint")
	case "breaking":
		return r.command(stdout, "buf", "breaking", "--against", against)
	}
	want := make(map[string][]byte)
	if len(protos) > 0 {
		want, err = r.generate(root, protos)
		if err != nil {
			return err
		}
	}
	actual, err := outputs(ctx, root)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if action == "check" {
		if !equalFiles(actual, want) {
			return errors.New("generated contracts differ; run make generate")
		}
		return nil
	}
	if equalFiles(actual, want) {
		return nil
	}
	return publish(root, want)
}

type runner struct {
	ctx            context.Context
	dir            string
	stdout, stderr io.Writer
}

func (r runner) command(stdout io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(r.ctx, name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = r.dir, stdout, r.stderr
	// Do not wait forever for inherited pipes after cancellation (for example
	// when Buf has started go run plugin processes).
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, errors.Join(err, r.ctx.Err()))
	}
	return nil
}

func (r runner) generate(root *os.Root, protos map[string][]byte) (map[string][]byte, error) {
	data, err := readFile(root, "go.mod")
	if err != nil {
		return nil, err
	}
	modulePath := modfile.ModulePath(data)
	if err := module.CheckPath(modulePath); err != nil {
		return nil, fmt.Errorf("project module: %w", err)
	}
	dir, err := os.MkdirTemp("", "nostosx-api-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	routes, options, err := configuration(r.ctx, root, protos)
	if err != nil {
		return nil, err
	}
	for name, data := range map[string][]byte{"routes.yaml": routes, "openapi.yaml": options} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return nil, err
		}
	}
	goTool := os.Getenv("GO")
	if goTool == "" {
		goTool = "go"
	}
	config := pluginConfig(goTool, modulePath, dir)
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	if err := r.command(r.stdout, "buf", "generate", "--template", string(encoded)); err != nil {
		return nil, err
	}
	generated, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer generated.Close()
	result, err := outputs(r.ctx, generated)
	if err != nil {
		return nil, err
	}
	const specification = "api/http/openapi.swagger.json"
	var document any
	decoder := json.NewDecoder(bytes.NewReader(result[specification]))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("generated OpenAPI: %w", err)
	}
	result[specification], err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	result[specification] = append(result[specification], '\n')
	return result, nil
}
