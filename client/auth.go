package client

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/server/types"
)

type AuthClient struct {
	*baseClient
}

// Login authenticates with the server and configures the client to use the received token
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
