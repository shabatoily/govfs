package types

import "github.com/meteormin/govfs/config"

type ConfigRes struct {
	AppName string `json:"app_name"`
	config.ServerConfig
}
