package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/config"
	"github.com/meteormin/govfs/server/types"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

const (
	DefaultServerURL = "http://localhost:3000"
)

var configPath string

type (
	ContextKeyAppInfo    struct{}
	ContextKeyUserConfig struct{}
)

type UserConfig struct {
	ServerURL string
	Username  string
	Password  string
	TokenInfo TokenInfo
}

type TokenInfo struct {
	types.TokenResponse
}

func (t TokenInfo) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return true
	}
	return t.ExpiresAt.Before(time.Now())
}

func GetUserConfig() (UserConfig, error) {
	var userConfig UserConfig

	file, err := os.Open(filepath.Join(configPath, "config"))
	if err != nil {
		return userConfig, err
	}
	defer file.Close()

	err = toml.NewDecoder(file).Decode(&userConfig)

	return userConfig, err
}

func SetUserConfig(u *UserConfig) error {
	file, err := os.Create(filepath.Join(configPath, "config"))
	if err != nil {
		return err
	}
	defer file.Close()

	return toml.NewEncoder(file).Encode(u)
}

func newConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Set configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return promptSetConfig(cmd, &UserConfig{})
		},
	}
}

func newInfoCommand() *cobra.Command {
	var verbose bool

	info := &cobra.Command{
		Use:   "info",
		Short: "Print system information",
		Run: func(c *cobra.Command, _ []string) {
			info := c.Context().Value(ContextKeyAppInfo{}).(*config.AppInfo)
			fmt.Printf("%s %s - %s\n", info.Name, info.Version, info.BuildTime)
			if verbose {
				b, err := toml.Marshal(info)
				if err != nil {
					fmt.Printf("\n[Info]\n%s\n", err.Error())
				} else {
					fmt.Printf("\n[Info]\n%s\n", string(b))
				}
			}
		},
	}

	info.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	return info
}

func NewRootCommand(appInfo *config.AppInfo) *cobra.Command {
	root := &cobra.Command{
		Use:   appInfo.Name,
		Short: appInfo.Name,
		Long:  appInfo.Description,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.WithValue(cmd.Context(), ContextKeyAppInfo{}, appInfo)

			if cmd.Name() == "info" {
				cmd.SetContext(ctx)
				return nil
			}

			if configPath == "" {
				baseDir, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				configPath = baseDir
			}

			configPath = filepath.Join(configPath, ".govfs")
			if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
				err = os.Mkdir(configPath, vfs.DefaultDirMode)
				if err != nil {
					return err
				}
			}

			if cmd.Name() == "config" {
				cmd.SetContext(ctx)
				return nil
			}

			u, err := GetUserConfig()
			if err != nil {
				if promptErr := promptSetConfig(cmd, &u); promptErr != nil {
					return promptErr
				}
			}

			ctx = context.WithValue(ctx, ContextKeyUserConfig{}, &u)

			cmd.SetContext(ctx)

			return nil
		},
	}

	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config file or directory (default: ~/.govfs/config)")

	root.AddCommand(newInfoCommand())

	root.AddCommand(newConfigCommand())

	return root
}

func promptSetConfig(cmd *cobra.Command, u *UserConfig) error {
	reader := bufio.NewReader(os.Stdin)

	cmd.Print("🔗 \033[36mEnter server URL:\033[0m\n   ")
	u.ServerURL, _ = reader.ReadString('\n')
	u.ServerURL = strings.TrimSpace(u.ServerURL)
	if u.ServerURL == "" {
		u.ServerURL = DefaultServerURL
	}

	cmd.Print("👤 \033[36mEnter username:\033[0m\n   ")
	u.Username, _ = reader.ReadString('\n')
	u.Username = strings.TrimSpace(u.Username)

	cmd.Print("🔑 \033[36mEnter password:\033[0m\n   ")

	err := disableEcho(cmd.Context())
	if err != nil {
		return err
	}

	u.Password, _ = reader.ReadString('\n')
	u.Password = strings.TrimSpace(u.Password)

	err = enableEcho(cmd.Context())
	if err != nil {
		return err
	}

	cmd.Println()
	cmd.Println("\n✨ \033[1m[Input Config]\033[0m")
	cmd.Println("   \033[34m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
	cmd.Printf("   📍 \033[1mServer URL\033[0m : %s\n", u.ServerURL)
	cmd.Printf("   🆔 \033[1mUsername\033[0m   : %s\n", u.Username)
	cmd.Println("   \033[34m━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\033[0m")
	cmd.Println()

	err = SetUserConfig(u)
	if err != nil {
		return err
	}

	cmd.Println("✅ \033[32mConfig saved successfully!\033[0m")

	return nil
}

func disableEcho(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "stty", "-echo")
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func enableEcho(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "stty", "echo")
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
