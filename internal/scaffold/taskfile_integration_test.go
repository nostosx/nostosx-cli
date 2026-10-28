//go:build integration

package scaffold_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/scaffold"
)

// Verify the public Task argument contract with real Task execution and a
// recording executable, without repeating generated-project build acceptance.
func TestTaskTestOptions(t *testing.T) {
	fixtureDir := t.TempDir()
	fixtureSource := filepath.Join(fixtureDir, "fixture.go")
	write(t, fixtureSource, `package main
import ("encoding/json"; "fmt"; "os")
func main() {
 data, err := json.Marshal(os.Args[1:]); if err != nil { panic(err) }
 fmt.Printf("NOSTOSX_TASK_ARGS=%s\n", data)
}
`)
	name := "go fixture"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fixture := filepath.Join(fixtureDir, name)
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	runProject(t, source, goExecutable(), "build", "-o", fixture, fixtureSource)
	dirs := map[string]string{"cli": source}
	for _, transport := range []string{"http", "grpc"} {
		o := options(t)
		o.Transport = transport
		if _, err := scaffold.New(t.Context(), o); err != nil {
			t.Fatal(err)
		}
		dirs[transport] = o.Output
	}
	for name, dir := range dirs {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name, option string
				race, cover  bool
				integration  bool
			}{
				{name: "plain"},
				{name: "coverage implies race", option: "COVER=true", race: true, cover: true},
				{name: "race", option: "RACE=true", race: true},
				{name: "integration", option: "INTEGRATION=true", integration: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					args := []string{"test", "GO=" + fixture}
					if tc.option != "" {
						args = append(args, tc.option)
					}
					calls := taskCalls(t, dir, args...)
					wantCalls := 1
					if tc.cover {
						wantCalls++
					}
					if len(calls) != wantCalls || len(calls[0]) == 0 || calls[0][0] != "test" ||
						slices.Contains(calls[0], "-race") != tc.race ||
						slices.Contains(calls[0], "-coverprofile=coverage.out") != tc.cover ||
						slices.Contains(calls[0], "integration") != tc.integration {
						t.Fatalf("test options: calls=%v", calls)
					}
					if tc.cover && !reflect.DeepEqual(calls[1], []string{"tool", "cover", "-func=coverage.out"}) {
						t.Fatalf("coverage report: %v", calls[1])
					}
				})
			}
		})
	}
	if calls := taskCalls(t, dirs["grpc"], "api", "NOSTOSX="+fixture); !reflect.DeepEqual(calls, [][]string{{"api", "lint", "--dir", "."}, {"api", "check", "--dir", "."}}) {
		t.Fatalf("default API task mutated or skipped checks: %v", calls)
	}
	if calls := taskCalls(t, dirs["grpc"], "api", "ACTION=generate", "NOSTOSX="+fixture); !reflect.DeepEqual(calls, [][]string{{"api", "generate", "--dir", "."}}) {
		t.Fatalf("explicit generation: %v", calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "task", "api", "ACTION=invalid", "NOSTOSX="+fixture)
	cmd.Dir = dirs["grpc"]
	if output, err := cmd.CombinedOutput(); err == nil || bytes.Contains(output, []byte("NOSTOSX_TASK_ARGS=")) {
		t.Fatalf("invalid action executed a tool: %v\n%s", err, output)
	}
}

func taskCalls(t *testing.T, dir string, args ...string) [][]string {
	t.Helper()
	output := runProject(t, dir, "task", args...)
	var calls [][]string
	for line := range bytes.SplitSeq(output, []byte("\n")) {
		data, ok := bytes.CutPrefix(line, []byte("NOSTOSX_TASK_ARGS="))
		if !ok {
			continue
		}
		var args []string
		if err := json.Unmarshal(data, &args); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, args)
	}
	return calls
}
