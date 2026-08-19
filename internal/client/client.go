// Package client는 govfs 서버와 통신하기 위한 API 클라이언트를 제공합니다.
package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/types"
)

// baseClient는 모든 세부 클라이언트(Auth, Cloud 등)에서 공통으로 사용하는 기본 클라이언트 구조체입니다.
type baseClient struct {
	mu    sync.RWMutex // 읽기/쓰기 잠금 제어
	c     *client.Client
	url   string
	token string
}

// SetToken은 서버 통신에 사용할 인증 토큰을 수동으로 설정합니다.
func (c *baseClient) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	if token != "" && token != "disabled" {
		c.c.AddHeader(fiber.HeaderAuthorization, "Bearer "+token)
	}
}

// SetClientID는 비동기 작업 완료 알림을 받을 SSE 클라이언트 ID를 설정합니다.
func (c *baseClient) SetClientID(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.c.AddHeader("X-Client-ID", id.String())
}

// Config는 서버로부터 전체 설정 정보를 조회합니다.
func (c *baseClient) Config(ctx context.Context) (types.ConfigRes, error) {
	res, err := c.c.Get("/config", client.Config{
		Ctx:    ctx,
		Header: map[string]string{"Content-Type": "application/json"},
	})
	if err != nil {
		return types.ConfigRes{}, err
	}
	var cfg types.ConfigRes
	if err := res.JSON(&cfg); err != nil {
		return types.ConfigRes{}, err
	}
	return cfg, nil
}

// Client는 모든 분산된 클라이언트 기능을 하나로 통합하는 메인 클라이언트 구조체입니다.
type Client struct {
	*baseClient
	auth  *AuthClient
	cloud *CloudClient
	sse   *SSEClient
	vfs   *VFSClient
}

// Auth는 인증 관련 통신을 담당하는 AuthClient를 반환합니다.
func (c *Client) Auth() *AuthClient {
	return c.auth
}

// Cloud는 클라우드 저장소 통신을 담당하는 CloudClient를 반환합니다.
func (c *Client) Cloud() *CloudClient {
	return c.cloud
}

// SSE는 이벤트 스트림 통신을 담당하는 SSEClient를 반환합니다.
func (c *Client) SSE() *SSEClient {
	return c.sse
}

// VFS는 가상 파일 시스템 연산을 담당하는 VFSClient를 반환합니다.
func (c *Client) VFS() *VFSClient {
	return c.vfs
}

// New는 주어진 URL을 기반으로 새로운 통합 API 클라이언트를 생성합니다.
func New(url string) *Client {
	c := client.New()
	c.SetBaseURL(url)
	base := &baseClient{c: c, url: url}
	return &Client{
		baseClient: base,
		auth:       &AuthClient{baseClient: base},
		cloud:      &CloudClient{baseClient: base},
		sse:        &SSEClient{baseClient: base},
		vfs:        &VFSClient{baseClient: base},
	}
}

// createJSONConfig는 데이터를 JSON으로 직렬화하여 클라이언트 요청 설정을 생성하는 헬퍼 함수입니다.
func createJSONConfig(ctx context.Context, data any) (client.Config, error) {
	jsonb, err := json.Marshal(data)
	if err != nil {
		return client.Config{}, err
	}
	return client.Config{
		Ctx:    ctx,
		Header: map[string]string{"Content-Type": "application/json"},
		Body:   jsonb,
	}, nil
}

// checkResponse는 응답 상태 코드를 확인하고 본문을 구조체로 언마샬링하는 공통 처리 함수입니다.
func checkResponse[T any](resp *client.Response, expectedStatus int, out *T) error {
	if resp.StatusCode() != expectedStatus {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}
	if out != nil {
		if err := resp.JSON(out); err != nil {
			return err
		}
	}
	return nil
}
