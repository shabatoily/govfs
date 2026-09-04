// Package main은 govfs 서버의 진입점입니다.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
	"github.com/kardianos/service"
	"github.com/shabatoily/govfs/cmd"
	_ "github.com/shabatoily/govfs/docs"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/server"
	"github.com/spf13/cobra"
)

const serviceShutdownTimeout = 5 * time.Second

var configPath = "config.toml"

type program struct {
	appInfo    config.AppInfo
	configPath string
	app        *fiber.App
	cancel     context.CancelFunc
	logger     service.Logger
}

// Start는 서비스 관리자를 차단하지 않고 HTTP 서버를 시작합니다.
func (p *program) Start(_ service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	app, port, err := p.initServer(ctx)
	if err != nil {
		cancel()
		return err
	}

	p.app = app
	p.cancel = cancel
	go func() {
		if err := app.Listen(":" + strconv.Itoa(port)); err != nil && !errors.Is(err, context.Canceled) {
			_ = p.logger.Error(err)
		}
	}()
	return nil
}

// Stop은 HTTP 서버와 연결된 저장소를 정상적으로 종료합니다.
func (p *program) Stop(_ service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.app == nil {
		return nil
	}
	return p.app.ShutdownWithTimeout(serviceShutdownTimeout)
}

func (p *program) runForeground() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, port, err := p.initServer(ctx)
	if err != nil {
		return err
	}
	return app.Listen(":"+strconv.Itoa(port), fiber.ListenConfig{GracefulContext: ctx})
}

func (p *program) initServer(ctx context.Context) (*fiber.App, int, error) {
	cfg, err := config.LoadWithViper(p.configPath, p.appInfo)
	if err != nil {
		return nil, 0, err
	}
	cfg.SetContext(ctx)

	app, err := server.Init(cfg)
	if err != nil {
		return nil, 0, err
	}
	return app, cfg.Server.Port, nil
}

// @title govfs
// @version 1.0.0
// @description govfs is a virtual file system server
// @contact.name meteormin
// @contact.email miniyu97@gmail.com
// @servers.url http://localhost:3000
// @servers.description Localhost
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer 토큰을 "Bearer {token}" 형식으로 입력합니다.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	_ = godotenv.Load()
	return newRootCommand(cmd.GetAppInfo()).Execute()
}

func newRootCommand(appInfo config.AppInfo) *cobra.Command {
	var prg *program
	var svc service.Service

	var root *cobra.Command
	root = &cobra.Command{
		Use:               appInfo.Name,
		Short:             appInfo.Description,
		Args:              cobra.NoArgs,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(command *cobra.Command, _ []string) error {
			path, err := absoluteConfigPath(configPath)
			if err != nil {
				return err
			}
			prg = &program{appInfo: appInfo, configPath: path}
			if command == root && service.Interactive() {
				return nil
			}

			svc, err = newService(prg)
			if err != nil {
				return err
			}
			prg.logger, err = svc.Logger(nil)
			return err
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			if service.Interactive() {
				return prg.runForeground()
			}
			return svc.Run()
		},
	}
	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "config file path")

	serviceCommand := &cobra.Command{
		Use:   "service",
		Short: "Manage the system service",
		Args:  cobra.NoArgs,
	}
	for _, action := range service.ControlAction {
		serviceCommand.AddCommand(&cobra.Command{
			Use:   action,
			Short: action + " the system service",
			Args:  cobra.NoArgs,
			RunE: func(_ *cobra.Command, _ []string) error {
				return service.Control(svc, action)
			},
		})
	}
	serviceCommand.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the system service status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			status, err := svc.Status()
			if err != nil {
				return err
			}
			command.Println(serviceStatusText(status))
			return nil
		},
	})

	root.AddCommand(serviceCommand)
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show govfs version",
		Args:  cobra.NoArgs,
		Run: func(c *cobra.Command, _ []string) {
			c.Println(appInfo.Name + " " + appInfo.Version)
		},
	})
	return root
}

func serviceStatusText(status service.Status) string {
	switch status {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

func newService(prg *program) (service.Service, error) {
	return service.New(prg, &service.Config{
		Name:        prg.appInfo.Name,
		DisplayName: prg.appInfo.Name,
		Description: prg.appInfo.Description,
		Arguments:   []string{"--config", prg.configPath},
		EnvVars:     serviceEnv(),
		Option: service.KeyValue{
			"UserService": true,
			"RunAtLoad":   true,
		},
	})
}

func absoluteConfigPath(path string) (string, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".govfs", "config.toml"), nil
	}
	return filepath.Abs(path)
}

func serviceEnv() map[string]string {
	env := make(map[string]string)
	for _, name := range []string{
		"SERVER_AUTH_ADMIN_USERNAME",
		"SERVER_AUTH_ADMIN_PASSWORD",
		"SERVER_AUTH_JWT_SECRET",
		"USERPROFILE",
	} {
		if value, ok := os.LookupEnv(name); ok {
			env[name] = value
		}
	}
	return env
}
