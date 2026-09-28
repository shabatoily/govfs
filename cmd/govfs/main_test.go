package main

import (
	"bytes"
	"testing"

	"github.com/kardianos/service"
	"github.com/shabatoily/govfs/internal/config"
)

func TestServiceCommands(t *testing.T) {
	root := newRootCommand(config.AppInfo{Name: "govfs"})
	for _, action := range []string{"install", "start", "stop", "restart", "uninstall", "status"} {
		command, _, err := root.Find([]string{"service", action})
		if err != nil {
			t.Errorf("service %s 명령 조회: %v", action, err)
			continue
		}
		if command.Name() != action {
			t.Errorf("service %s 명령 대신 %s 조회", action, command.Name())
		}
	}
}

func TestServiceStatusText(t *testing.T) {
	tests := map[service.Status]string{
		service.StatusUnknown: "unknown",
		service.StatusRunning: "running",
		service.StatusStopped: "stopped",
	}
	for status, want := range tests {
		if got := serviceStatusText(status); got != want {
			t.Errorf("serviceStatusText(%d) = %q, 기대값 %q", status, got, want)
		}
	}
}

func TestVersionWithoutConfig(t *testing.T) {
	for _, arg := range []string{"--version", "version"} {
		t.Run(arg, func(t *testing.T) {
			root := newRootCommand(config.AppInfo{Name: "govfs", Version: "v1.2.3"})
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)
			root.SetArgs([]string{"--config", "/dev/null/missing", arg})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			want := "govfs version v1.2.3\n"
			if arg == "version" {
				want = "govfs v1.2.3\n"
			}
			if got := output.String(); got != want {
				t.Fatalf("version output = %q, want %q", got, want)
			}
		})
	}
}
