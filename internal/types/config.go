// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/internal/config"
)

// ConfigRes는 서버 설정을 클라이언트에 전달하기 위한 구조체입니다.
type ConfigRes struct {
	AppName string
	config.ServerConfig
}

func (cfg *ConfigRes) MarshalJSON() ([]byte, error) {
	type FiberAlias struct {
		fiber.Config
		ServicesStartupContextProvider  any `json:",omitempty"`
		ServicesShutdownContextProvider any `json:",omitempty"`
	}

	type ServerAlias struct {
		config.ServerConfig
		Fiber FiberAlias `json:"fiber"`
	}

	alias := ServerAlias{
		ServerConfig: cfg.ServerConfig,
		Fiber: FiberAlias{
			Config: cfg.Fiber,
		},
	}

	// 표준 json 라이브러리를 사용해야 함
	return json.Marshal(struct {
		AppName string `json:"appName"`
		ServerAlias
	}{
		AppName:     cfg.AppName,
		ServerAlias: alias,
	})
}
