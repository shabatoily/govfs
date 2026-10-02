package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	vfs "github.com/shabatoily/govfs"
	"github.com/spf13/viper"
)

const defaultConfigName = "config"

// LoadWithViper는 지정된 파일 경로에서 Viper 라이브러리를 사용하여 설정을 로드하고
// 환경 변수 및 기본값과 병합하여 검증된 최종 설정 객체를 반환합니다.
func LoadWithViper(in string, appInfo AppInfo) (*Config, error) {
	if in == "" {
		in = defaultConfigName
	}

	in = resolveConfigPath(in)

	// 로딩별 인스턴스를 사용해 설정과 기본값이 다른 서버로 전파되지 않도록 합니다.
	v := viper.New()
	v.SetConfigFile(in)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.SetDefault("vfs.idleTimeout", DefaultConfig.VFS.IdleTimeout)
	err := v.ReadInConfig()
	if err != nil {
		return nil, err
	}

	cfg := Config{}
	err = v.Unmarshal(&cfg)
	if err != nil {
		return nil, err
	}
	cfg.App = appInfo
	err = resolveConfig(&cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func resolveConfigPath(in string) string {
	if filepath.Ext(in) != "" {
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
	return os.MkdirAll(filepath.Dir(path), vfs.DefaultDirMode)
}
