package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-json"
	"github.com/spf13/viper"
)

var defaultConfigName = "config"

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
	if cfg.Server.Auth.Enabled && cfg.Server.Auth.JWTSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		cfg.Server.Auth.JWTSecret = hex.EncodeToString(b)
	}

	if cfg.VFS.Driver.Type == "" {
		cfg.VFS.Driver = DefaultConfig.VFS.Driver
	}
	if cfg.Server.JSONEncoder == nil {
		cfg.Server.JSONEncoder = json.Marshal
	}
	if cfg.Server.JSONDecoder == nil {
		cfg.Server.JSONDecoder = json.Unmarshal
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
	if cfg.VFS.Logger.Path != "" {
		err := mkdirAll(cfg.VFS.Logger.Path)
		if err != nil {
			return err
		}
	}
	return nil
}

func mkdirAll(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
		if err != nil {
			return err
		}
	}
	return nil
}

func setConfigFromViper(cfg *Config, appInfo AppInfo) error {
	if err := viper.Unmarshal(&cfg); err != nil {
		return err
	}

	cfg.App = appInfo
	cfg.Server.AppName = appInfo.Name + " v" + appInfo.Version

	return nil
}
