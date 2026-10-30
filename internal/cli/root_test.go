package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/cli"
)

func TestHelpAndVersion(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"--help", "--version"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			cmd := cli.New("test-version")
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs([]string{arg})
			if err := cmd.ExecuteContext(t.Context()); err != nil {
				t.Fatal(err)
			}
			if output.Len() == 0 || (arg == "--version" && !strings.Contains(output.String(), "test-version")) {
				t.Fatalf("output=%s", &output)
			}
		})
	}
}

func TestNewAndAdd(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "project with spaces")
	for _, args := range [][]string{
		{"new", "shop", "--module", "example.com/shop", "--layout", "microservices", "--transport", "http,grpc", "--with", "agentx", "--output", dir},
		{"add", "context", "order", "--dir", dir},
		{"add", "service", "order", "--dir", dir},
		{"add", "api", "--context", "order", "create-order", "--service", "order", "--method", "POST", "--path", "/v1/orders", "--dir", dir},
		{"add", "api", "--context", "order", "ping", "--transport", "grpc", "--kind", "query", "--service", "order", "--dir", dir},
		{"api", "scaffold", "--service", "order", "--dir", dir},
	} {
		cmd := cli.New("test")
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs(args)
		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "services", "order", "cmd", "order", "main.go")); err != nil {
		t.Fatal(err)
	}
}

func TestErrorsAreReportedOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		code int
	}{
		{"missing argument", []string{"new"}, 2},
		{"unknown flag", []string{"new", "shop", "--no-such-flag"}, 2},
		{"unknown feature", []string{"new", "shop", "--module", "example.com/shop", "--with", "unknown"}, 2},
		{"missing module", []string{"new", "shop"}, 2},
		{"missing API flags", []string{"add", "api", "--context", "order", "create-order"}, 2},
		{"missing scaffold service", []string{"api", "scaffold"}, 2},
		{"partial HTTP binding", []string{"add", "api", "--context", "order", "ping", "--service", "order", "--method", "GET"}, 2},
		{"extra doctor argument", []string{"doctor", "unexpected"}, 2},
		{"missing contract baseline", []string{"api", "breaking"}, 2},
		{"empty contract baseline", []string{"api", "breaking", "--against="}, 2},
		{"blank contract baseline", []string{"api", "breaking", "--against", " \t\n", "--dir", filepath.Join(t.TempDir(), "missing")}, 2},
		{"extra contract argument", []string{"api", "generate", "unexpected"}, 2},
		{"invalid API method", []string{"add", "api", "--context", "order", "create-order", "--service", "order", "--method", "INVALID", "--path", "/v1/orders"}, 2},
		{"missing project", []string{"add", "context", "order", "--dir", filepath.Join(t.TempDir(), "missing")}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := cli.New("test")
			var output, stderr bytes.Buffer
			cmd.SetArgs(tc.args)
			cmd.SetOut(&output)
			cmd.SetErr(&stderr)
			if got := cli.Execute(cmd); got != tc.code {
				t.Fatalf("code=%d want %d", got, tc.code)
			}
			if output.Len() != 0 || strings.Count(stderr.String(), "nostosx:") != 1 || strings.Contains(stderr.String(), "Usage:") {
				t.Fatalf("stdout=%s stderr=%s", &output, &stderr)
			}
		})
	}
}

func TestDoctorJSONFailure(t *testing.T) {
	t.Parallel()
	cmd := cli.New("test-version")
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"doctor", "--json", "--dir", filepath.Join(t.TempDir(), "missing")})
	if code := cli.Execute(cmd); code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var report struct {
		Version string `json:"version"`
		Checks  []struct {
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, &out)
	}
	if report.Version != "test-version" || len(report.Checks) == 0 || strings.Count(stderr.String(), "nostosx:") != 1 {
		t.Fatalf("report=%+v stderr=%s", report, &stderr)
	}
}
