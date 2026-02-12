package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/meteormin/go-vfs/config"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type Commands struct {
	cmd *cobra.Command
}

func NewCommands(cmd *cobra.Command) Commands {
	return Commands{
		cmd: cmd,
	}
}

func (c Commands) Append(cmd ...*cobra.Command) *cobra.Command {
	for _, v := range cmd {
		c.cmd.AddCommand(v)
	}

	return c.cmd
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

	var verbose bool
	info := &cobra.Command{
		Use:   "info",
		Short: "Print the version number",
		Run: func(c *cobra.Command, _ []string) {
			cfg := c.Context().Value(config.ContextKeyConfig{}).(*config.Config)
			fmt.Printf("%s %s - %s\n", cfg.App.Name, cfg.App.Version, cfg.App.BuildTime)
			if verbose {
				if build, err := toml.Marshal(cfg.App.BuildInfo); err == nil {
					fmt.Printf("\n[Build Info]\n%s\n", string(build))
				}
			}
		},
	}
	info.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	root.AddCommand(info)

	return root
}
