package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/meteormin/govfs/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newInfoCommand() *cobra.Command {
	var verbose bool

	info := &cobra.Command{
		Use:   "info",
		Short: "Print system information",
		Run: func(c *cobra.Command, _ []string) {
			cfg := c.Context().Value(config.ContextKeyConfig{}).(*config.Config)
			fmt.Printf("%s %s - %s\n", cfg.App.Name, cfg.App.Version, cfg.App.BuildTime)
			if verbose {
				if b, err := toml.Marshal(cfg); err == nil {
					fmt.Printf("\n[Config]\n%s\n", string(b))
				}
			}
		},
	}

	info.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	return info
}

func NewRootCommand(appInfo config.AppInfo) *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   appInfo.Name,
		Short: appInfo.Name,
		Long:  appInfo.Description,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadWithViper(configPath, appInfo)
			if err != nil {
				return err
			}
			ctx := context.WithValue(cmd.Context(), config.ContextKeyConfig{}, cfg)
			cmd.SetContext(ctx)
			return nil
		},
	}

	configFlagUsage := fmt.Sprintf("path to config file (supported exts: %q",
		strings.Join(viper.SupportedExts, ","))

	root.PersistentFlags().StringVarP(&configPath, "config", "c", "", configFlagUsage)

	root.AddCommand(newInfoCommand())

	return root
}
