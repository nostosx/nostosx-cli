package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/nostosx/nostosx-cli/internal/cli"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx)
	stop()
	os.Exit(code)
}

func run(ctx context.Context) int {
	info, _ := debug.ReadBuildInfo()
	cmd := cli.New(buildVersion(info))
	cmd.SetContext(ctx)
	return cli.Execute(cmd)
}

func buildVersion(info *debug.BuildInfo) string {
	if version != "dev" {
		return version
	}
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
