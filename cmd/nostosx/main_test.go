package main

import (
	"runtime/debug"
	"testing"
)

func TestBuildVersion(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	for _, tt := range []struct {
		name, injected string
		info           *debug.BuildInfo
		want           string
	}{
		{name: "source build", injected: "dev", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, want: "dev"},
		{name: "unknown build", injected: "dev", want: "dev"},
		{name: "empty metadata", injected: "dev", info: &debug.BuildInfo{}, want: "dev"},
		{name: "go install", injected: "dev", info: &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}}, want: "v1.0.0"},
		{name: "release binary", injected: "v1.0.1", info: &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0"}}, want: "v1.0.1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			version = tt.injected
			if got := buildVersion(tt.info); got != tt.want {
				t.Fatalf("version=%q, want %q", got, tt.want)
			}
		})
	}
}
