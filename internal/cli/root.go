// Package cli는 govfs CLI 도구의 핵심 로직과 커맨드 구조를 정의합니다.
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

	"github.com/pelletier/go-toml/v2"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/client"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/spf13/cobra"
)

const (
	// defaultServerURL은 서버 접속 시 사용되는 기본 주소입니다.
	defaultServerURL = "http://localhost:3000"
)

var configPath string

// ContextKeyAppInfo 및 ContextKeyUserConfig는 Cobra 커맨드 컨텍스트에서 정보를 저장하기 위한 키 타입입니다.
type (
	ContextKeyAppInfo    struct{}
	ContextKeyUserConfig struct{}
)

// UserConfig는 CLI 클라이언트가 서버에 접속하기 위해 필요한 사용자 설정을 정의합니다.
type UserConfig struct {
	ServerURL string    // 서버 접속 주소
	Username  string    // 사용자 이름
	Password  string    // 사용자 비밀번호
	TokenInfo TokenInfo // 발급받은 인증 토큰 정보
}

// TokenInfo는 서버로부터 발급받은 토큰 응답 정보를 포함합니다.
type TokenInfo struct {
	types.TokenRes
}

func (t TokenInfo) IsExpired() bool {
	// 토큰 만료 시간이 설정되지 않은 경우에는 만료된 것으로 간주합니다.
	if t.ExpiresAt.IsZero() {
		return true
	}
	return t.ExpiresAt.Before(time.Now())
}

// GetUserConfig는 로컬 파일 시스템에서 사용자 설정 파일을 읽어 반환합니다.
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

// SetUserConfig는 사용자 설정을 로컬 파일 시스템에 저장합니다.
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

// NewRootCommand는 govfs CLI의 최상위(Root) 커맨드를 생성하고 초기화합니다.
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

			configPath = filepath.Join(configPath, ".govfs")
			if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
				err = os.Mkdir(configPath, vfs.DefaultDirMode)
				if err != nil {
					return err
				}
			}

			if cmd.Name() == "config" || (cmd.Name() == "info" && !cmd.Flag("verbose").Changed) {
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
		u.ServerURL = defaultServerURL
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
