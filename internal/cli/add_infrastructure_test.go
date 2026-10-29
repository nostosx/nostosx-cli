package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nostosx/nostosx-cli/internal/cli"
)

func TestInfrastructureCommandsAreOffline(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := filepath.Join(t.TempDir(), "project with spaces")
	for _, args := range [][]string{
		{"new", "shop", "--module", "example.com/shop", "--with", "agentx,mysql,kafka", "--output", dir},
		{"add", "infrastructure", "postgresql", "--dir", dir},
		{"add", "infrastructure", "nats", "--dir", dir},
	} {
		cmd := cli.New("test")
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.ExecuteContext(t.Context()); err != nil {
			t.Fatal(args, err)
		}
	}
	code, err := os.ReadFile(filepath.Join(dir, "app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(code), "mysqlinfra.New(") || !strings.Contains(string(code), "kafkainfra.New(") || strings.Contains(string(code), "natsinfra.New(") {
		t.Fatal("new/add assembly boundary changed", string(code))
	}
}
