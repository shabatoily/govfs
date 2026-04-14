// Package types는 서버 전반에서 사용되는 데이터 구조를 정의합니다.
package types

import "github.com/meteormin/govfs/config"

// ConfigRes는 서버 설정을 클라이언트에 전달하기 위한 구조체입니다.
type ConfigRes struct {
	AppName string `json:"app_name"`
	config.ServerConfig
}
