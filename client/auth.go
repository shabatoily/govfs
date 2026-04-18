// Package client는 govfs 서버와 통신하기 위한 API 클라이언트를 제공합니다.
package client

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/server/types"
)

// AuthClient는 사용자 인증 및 세션 관리 API 통신을 담당하는 클라이언트입니다.
type AuthClient struct {
	*baseClient
}

// Login은 서버에 인증을 요청하고, 성공 시 발급받은 토큰을 클라이언트에 설정합니다.
func (c *AuthClient) Login(username, password string) (types.TokenResponse, error) {
	var res types.TokenResponse

	req := types.LoginRequest{
		Username: username,
		Password: password,
	}

	cfg, err := createJSONConfig(req)
	if err != nil {
		return res, err
	}

	resp, err := c.c.Post("/auth/login", cfg)
	if err != nil {
		return res, err
	}

	status := resp.StatusCode()
	if status != fiber.StatusOK && status != fiber.StatusNoContent {
		return res, fmt.Errorf("login failed: %v", resp.StatusCode())
	} else if status == fiber.StatusNoContent {
		return res, nil
	}

	if err := resp.JSON(&res); err != nil {
		return res, err
	}

	c.SetToken(res.Token)
	// If no token but OK, it could be "Auth is disabled"
	return res, nil
}

// Me는 현재 로그인된 사용자의 정보를 조회하고 토큰을 갱신합니다.
func (c *AuthClient) Me() (types.TokenResponse, error) {
	var res types.TokenResponse

	resp, err := c.c.Get("/auth/me")
	if err != nil {
		return res, err
	}

	status := resp.StatusCode()
	if status != fiber.StatusOK && status != fiber.StatusNoContent {
		return res, fmt.Errorf("not logged in: %v", resp.StatusCode())
	} else if status == fiber.StatusNoContent {
		return res, nil
	}

	if err := resp.JSON(&res); err != nil {
		return res, err
	}

	c.SetToken(res.Token)

	return res, nil
}
