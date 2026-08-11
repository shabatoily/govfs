// Package config는 애플리케이션, 서버, 가상 파일 시스템(VFS), 클라우드 등의 시스템 설정 구조를 정의합니다.
package config

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/gofiber/fiber/v3"
	fiberLog "github.com/gofiber/fiber/v3/log"
	"github.com/meteormin/govfs/docs"
	"github.com/meteormin/govfs/internal/cloud"
	"github.com/meteormin/govfs/pkg/drivers"
	"github.com/meteormin/govfs/pkg/drivers/badger"
	vfsLog "github.com/meteormin/govfs/pkg/log"
)

// AppInfo는 애플리케이션의 이름, 버전, 빌드 시간 등의 메타데이터를 보관하는 구조체입니다.
type AppInfo struct {
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	BuildTime   string           `json:"buildTime"`
	Description string           `json:"description"`
	BuildInfo   *debug.BuildInfo `json:"buildInfo"`
}

// ServerConfig는 HTTP 서버 구동 및 미들웨어 관련 설정을 포함합니다.
type ServerConfig struct {
	Auth AuthConfig `json:"auth"`

	Context context.Context `json:"-"`

	Fiber fiber.Config `json:"fiber"`

	Host string `json:"host"`

	Logger FiberLoggerConfig `json:"logger"`

	Middlewares MiddlewareConfig `json:"middlewares"`

	Port int `json:"port"`

	WebUI WebUIConfig `json:"webui"`
}

// FiberLoggerConfig는 Fiber 웹 프레임워크 로거의 출력 파일 경로 및 로그 레벨을 정의합니다.
type FiberLoggerConfig struct {
	Path          string         `json:"path"`
	Level         fiberLog.Level `json:"level"`
	AccessLogPath string         `json:"accessLogPath"`
}

// AuthConfig는 서버 인증 기능의 사용 여부와 계정, JWT 암호화 정보를 설정합니다.
type AuthConfig struct {
	Enabled  bool      `json:"enabled"`
	Username string    `json:"username"`
	Password string    `json:"-"`
	JWT      JWTConfig `json:"jwt"`
}

// MiddlewareConfig는 서버에서 제공하는 부가 기능(디버그, 환경 변수 등) 미들웨어 활성화 상태를 제어합니다.
type MiddlewareConfig struct {
	Config  bool `json:"config"`
	Envvar  bool `json:"envvar"`
	Expvar  bool `json:"expvar"`
	Pprof   bool `json:"pprof"`
	Route   bool `json:"route"`
	Swagger bool `json:"swagger"`
}

// WebUIConfig는 내장 웹 페이지 노출 여부를 설정합니다.
type WebUIConfig struct {
	Enabled bool `json:"enabled"`
}

// JWTConfig는 JWT 기반 인증용 비밀키와 만료 시간을 설정합니다.
type JWTConfig struct {
	Secret string        `json:"-"`
	Exp    time.Duration `json:"exp"`
}

// VfsConfig는 VFS에서 사용할 백업, 로깅 경로 및 실제 데이터를 저장할 드라이버를 설정합니다.
type VfsConfig struct {
	Driver drivers.Config `json:"driver"`
	Logger vfsLog.Config  `json:"logger"`
}

// CloudConfig는 외부 클라우드 백엔드 연동을 위한 상세 설정을 포함합니다.
type CloudConfig struct {
	cloud.Config
}

// Config는 govfs 서비스 전체를 구성하는 최상위 설정 구조체입니다.
type Config struct {
	ctx    context.Context `json:"-"`
	App    AppInfo         `json:"app"`
	Server ServerConfig    `json:"server"`
	VFS    VfsConfig       `json:"vfs"`
	Cloud  CloudConfig     `json:"cloud"`
}

// SetContext는 설정 객체 및 하위 속성들에 컨텍스트(context.Context)를 주입합니다.
func (c *Config) SetContext(ctx context.Context) {
	c.ctx = ctx
	c.Server.Context = ctx
	c.VFS.Driver.Badger.Context = ctx
}

// Context는 현재 설정 객체에 주입된 컨텍스트를 반환합니다.
func (c *Config) Context() context.Context {
	return c.ctx
}

// DefaultConfig는 기본적인 운용이 가능하도록 미리 정의된 초기 설정 파일 템플릿 역할을 합니다.
var DefaultConfig = &Config{
	Cloud: CloudConfig{
		Config: cloud.Config{
			ClientType: "googleDrive",
		},
	},
	Server: ServerConfig{
		Port: 3000,
	},
	VFS: VfsConfig{
		Driver: drivers.Config{
			Type: drivers.DriverTypeBadger,
			Badger: badger.Config{
				Path: "./data",
			},
		},
	},
}

// SetSwaggerInfo는 최상위 설정 객체를 읽고 Swagger 환경 전역 변수에 API 문서용 메타 정보를 적용합니다.
func SetSwaggerInfo(cfg *Config) {
	docs.SwaggerInfo.Title = cfg.App.Name
	docs.SwaggerInfo.Description = cfg.App.Description
	docs.SwaggerInfo.Version = cfg.App.Version
	docs.SwaggerInfo.Host = fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
}
