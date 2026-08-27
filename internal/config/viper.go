package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	vfs "github.com/shabatoily/govfs"
	"github.com/spf13/viper"
)

var defaultConfigName = "config"

// LoadWithViper는 지정된 파일 경로에서 Viper 라이브러리를 사용하여 설정을 로드하고
// 환경 변수 및 기본값과 병합하여 검증된 최종 설정 객체를 반환합니다.
func LoadWithViper(in string, appInfo AppInfo) (*Config, error) {
	if in == "" {
		in = defaultConfigName
	}

	in = resolveConfigPath(in)

	// 환경 변수 치환
	viper.SetConfigType(filepath.Ext(in)[1:]) // yml, yaml, json, toml 등
	viper.SetConfigFile(in)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("vfs.idleTimeout", DefaultConfig.VFS.IdleTimeout)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}

	cfg := Config{}
	err = setConfigFromViper(&cfg, appInfo)
	if err != nil {
		return nil, err
	}
	err = resolveConfig(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func resolveConfigPath(in string) string {
	if strings.Contains(in, ".") {
		return in
	}

	for _, ext := range viper.SupportedExts {
		if _, err := os.Stat(in + "." + ext); err == nil {
			return in + "." + ext
		}
	}

	return in
}

func resolveConfig(cfg *Config) error {
	paths := []*string{
		&cfg.Server.Logger.Path,
		&cfg.Server.Logger.AccessLogPath,
		&cfg.VFS.Logger.Path,
		&cfg.VFS.Driver.Badger.Path,
		&cfg.VFS.Driver.LocalStorage.Path,
	}
	for _, path := range paths {
		resolved, err := expandHomePath(*path)
		if err != nil {
			return err
		}
		*path = resolved
	}

	cfg.Server.Fiber = DefaultConfig.Server.Fiber
	if cfg.Server.Fiber.AppName == "" {
		cfg.Server.Fiber.AppName = cfg.App.Name + " " + cfg.App.Version
	}
	if cfg.Server.Auth.JWT.Secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		cfg.Server.Auth.JWT.Secret = hex.EncodeToString(b)
	}
	if cfg.Server.Port < 0 {
		cfg.Server.Port = DefaultConfig.Server.Port
	}
	if cfg.Server.Logger.Path != "" {
		err := mkdirAll(cfg.Server.Logger.Path)
		if err != nil {
			return err
		}
	}

	if cfg.VFS.Driver.Type == "" {
		cfg.VFS.Driver = DefaultConfig.VFS.Driver
	}
	if cfg.VFS.Logger.Path != "" {
		err := mkdirAll(cfg.VFS.Logger.Path)
		if err != nil {
			return err
		}
	}

	return nil
}

func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func mkdirAll(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(filepath.Dir(path), vfs.DefaultDirMode)
		if err != nil {
			return err
		}
	}
	return nil
}

func setConfigFromViper(cfg *Config, appInfo AppInfo) error {
	if err := viper.Unmarshal(cfg); err != nil {
		return err
	}

	cfg.App = appInfo

	return nil
}
