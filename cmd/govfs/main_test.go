package main

import (
	"testing"

	"github.com/shabatoily/govfs/internal/config"
)

func TestServiceCommands(t *testing.T) {
	root := newRootCommand(config.AppInfo{Name: "govfs"})
	for _, action := range []string{"install", "start", "stop", "restart", "uninstall"} {
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
