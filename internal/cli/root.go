// Package cli는 govfs CLI 도구의 핵심 로직과 커맨드 구조를 정의합니다.
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

// UserConfig는 CLI 클라이언트가 서버에 접속하기 위해 필요한 사용자 설정을 정의합니다.
type UserConfig struct {
	ServerURL string    // 서버 접속 주소
	Username  string    // 사용자 이름
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
func GetUserConfig(cmd *cobra.Command) (UserConfig, error) {
	var userConfig UserConfig
	configPath, err := userConfigDir(cmd)
	if err != nil {
		return userConfig, err
	}

	file, err := os.Open(filepath.Join(configPath, "config"))
	if err != nil {
		return userConfig, err
	}
	defer file.Close()

	err = toml.NewDecoder(file).Decode(&userConfig)

	return userConfig, err
}

// setUserConfig는 사용자 설정을 로컬 파일 시스템에 저장합니다.
func setUserConfig(cmd *cobra.Command, u *UserConfig) error {
	configPath, err := userConfigDir(cmd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configPath, vfs.DefaultDirMode); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(configPath, "config"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}

	return toml.NewEncoder(file).Encode(u)
}

func newLoginCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Log in to a govfs server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return promptLogin(cmd)
		},
	}
}

func newInfoCommand(appInfo config.AppInfo) *cobra.Command {
	var verbose bool

	info := &cobra.Command{
		Use:   "info",
		Short: "Print system information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !verbose {
				cmd.Printf("%s %s - %s\n", appInfo.Name, appInfo.Version, appInfo.BuildTime)
				return nil
			}

			b, err := toml.Marshal(appInfo)
			if err != nil {
				return err
			}

			cmd.Printf("\n[Client]\n%s\n", string(b))

			return nil
		},
	}

	info.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	return info
}

// NewRootCommand는 govfs CLI의 최상위(Root) 커맨드를 생성하고 초기화합니다.
func NewRootCommand(appInfo config.AppInfo) *cobra.Command {
	root := &cobra.Command{
		Use:     appInfo.Name,
		Short:   appInfo.Name,
		Long:    appInfo.Description,
		Version: appInfo.Version,
	}

	root.PersistentFlags().StringP("config", "c", "", "base directory for session storage (default: home directory; uses .govfs/config)")

	root.AddCommand(newInfoCommand(appInfo))

	root.AddCommand(newLoginCommand())

	return root
}

func promptLogin(cmd *cobra.Command) error {
	reader := bufio.NewReader(os.Stdin)
	u := UserConfig{}

	cmd.Print("🔗 \033[36mEnter server URL:\033[0m\n   ")
	var err error
	u.ServerURL, err = reader.ReadString('\n')
	if err != nil {
		return err
	}
	u.ServerURL = strings.TrimSpace(u.ServerURL)
	if u.ServerURL == "" {
		u.ServerURL = defaultServerURL
	}

	cmd.Print("👤 \033[36mEnter username:\033[0m\n   ")
	u.Username, err = reader.ReadString('\n')
	if err != nil {
		return err
	}
	u.Username = strings.TrimSpace(u.Username)

	cmd.Print("🔑 \033[36mEnter password:\033[0m\n   ")

	password, err := readPassword(cmd.Context(), reader)
	if err != nil {
		return err
	}

	cmd.Println()
	c := client.New(u.ServerURL)
	token, err := c.Auth().Login(cmd.Context(), u.Username, password)
	if err != nil {
		return err
	}
	u.TokenInfo = TokenInfo{TokenRes: token}
	err = setUserConfig(cmd, &u)
	if err != nil {
		return err
	}

	cmd.Println("✅ \033[32mLogin successful!\033[0m")

	return nil
}

// userConfigDir는 플래그 입력을 변경하지 않고 세션 저장 경로를 계산합니다.
func userConfigDir(cmd *cobra.Command) (string, error) {
	baseDir, err := cmd.Root().PersistentFlags().GetString("config")
	if err != nil {
		return "", err
	}
	if baseDir == "" {
		baseDir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(baseDir, ".govfs"), nil
}

// NewAuthenticatedClient는 저장된 세션을 읽고 서버에서 인증을 확인합니다.
func NewAuthenticatedClient(cmd *cobra.Command) (*client.Client, error) {
	u, err := GetUserConfig(cmd)
	if err != nil {
		return nil, fmt.Errorf("not logged in: run govfs login: %w", err)
	}
	if u.TokenInfo.IsExpired() {
		return nil, errors.New("session expired: run govfs login")
	}
	c := client.New(u.ServerURL)
	c.SetToken(u.TokenInfo.Token)
	if _, err := c.Auth().Me(cmd.Context()); err != nil {
		return nil, fmt.Errorf("session invalid: run govfs login: %w", err)
	}
	return c, nil
}

func readPassword(ctx context.Context, reader *bufio.Reader) (password string, err error) {
	cmd := exec.CommandContext(ctx, "stty", "-g")
	cmd.Stdin = os.Stdin
	state, err := cmd.Output()
	if err != nil {
		return "", err
	}
	cmd = exec.CommandContext(ctx, "stty", "-echo")
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return "", err
	}
	defer func() {
		// 로그인 취소 여부와 무관하게 기존 터미널 상태를 복구합니다.
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		restore := exec.CommandContext(restoreCtx, "stty", strings.TrimSpace(string(state))) //nolint:gosec // 인자는 stty -g로 읽은 기존 터미널 상태입니다.
		restore.Stdin = os.Stdin
		err = errors.Join(err, restore.Run())
	}()
	password, err = reader.ReadString('\n')
	return strings.TrimSpace(password), err
}
