package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-json"
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
	ServerURL string    `json:"serverURL"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	TokenInfo TokenInfo `json:"tokenInfo"`
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

	file, err := os.Open(filepath.Join(configPath, "config.json"))
	if err != nil {
		return userConfig, err
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&userConfig)

	return userConfig, err
}

func SetUserConfig(u UserConfig) error {
	file, err := os.Create(filepath.Join(configPath, "config.json"))
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(&u)
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

func NewRootCommand(appInfo config.AppInfo) *cobra.Command {
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

			configPath = filepath.Join(configPath, ".govfs")
			if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
				err = os.Mkdir(configPath, vfs.DefaultFileMode)
				if err != nil {
					return err
				}
			}

			u, err := GetUserConfig()
			if err != nil {
				u.ServerURL = DefaultServerURL
				SetUserConfig(u)
			}

			ctx = context.WithValue(ctx, ContextKeyUserConfig{}, u)

			cmd.SetContext(ctx)

			return nil
		},
	}

	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", "path to config.json file")

	root.AddCommand(newInfoCommand())

	return root
}
