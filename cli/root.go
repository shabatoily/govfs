package cli

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/client"
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

	//nolint:gosec // Password is not hardcoded, it is stored in the config file
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := cmd.Context().Value(ContextKeyAppInfo{}).(*config.AppInfo)
			if !verbose {
				cmd.Printf("%s %s - %s\n", info.Name, info.Version, info.BuildTime)
				return nil
			}

			b, err := toml.Marshal(info)
			if err != nil {
				return err
			}

			cmd.Printf("\n[Client]\n%s\n", string(b))

			u, err := GetUserConfig()
			if err != nil {
				return err
			}

			c := client.New(u.ServerURL)
			t, err := c.Auth().Login(u.Username, u.Password)
			if err != nil {
				return err
			}
			c.SetToken(t.Token)

			cfg, err := c.Config()
			if err != nil {
				return err
			}

			b, err = toml.Marshal(cfg)
			if err != nil {
				return err
			}

			cmd.Printf("\n[Server]\n%s\n", string(b))

			return nil
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

			if configPath == "" {
				baseDir, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				configPath = baseDir
			}

			if cmd.Name() == "config" || (cmd.Name() == "info" && !cmd.Flag("verbose").Changed) {
				cmd.SetContext(ctx)
				return nil
			}

			configPath = filepath.Join(configPath, ".govfs")
			if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
				err = os.Mkdir(configPath, vfs.DefaultDirMode)
				if err != nil {
					return err
				}
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
