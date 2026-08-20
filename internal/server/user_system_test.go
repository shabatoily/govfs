package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/shabatoily/govfs/internal/config"
	"github.com/shabatoily/govfs/internal/types"
	"github.com/shabatoily/govfs/pkg/drivers"
	"github.com/shabatoily/govfs/pkg/drivers/badger"
)

func TestUserSystemAdminBoundary(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Auth: config.AuthConfig{
				Admin: config.AdminConfig{Username: "admin", Password: "password"},
				JWT:   config.JWTConfig{Secret: "test-secret", Exp: time.Hour},
			},
		},
		VFS: config.VfsConfig{Driver: drivers.Config{
			Type:   drivers.DriverTypeBadger,
			Badger: badger.Config{Path: filepath.Join(t.TempDir(), "drives")},
		}},
	}
	cfg.SetContext(context.Background())
	app, err := Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown() })

	adminToken := loginToken(t, app, "admin", "password")
	createReq := request(t, http.MethodPost, "/admin/users", map[string]any{
		"username": "member", "password": "password", "role": "user",
	}, adminToken)
	createRes, err := app.Test(createReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = createRes.Body.Close()
	if createRes.StatusCode != http.StatusCreated {
		t.Fatalf("사용자 생성 상태 = %d", createRes.StatusCode)
	}

	memberToken := loginToken(t, app, "member", "password")
	listRes, err := app.Test(request(t, http.MethodGet, "/admin/users", nil, memberToken))
	if err != nil {
		t.Fatal(err)
	}
	_ = listRes.Body.Close()
	if listRes.StatusCode != http.StatusForbidden {
		t.Fatalf("일반 사용자 관리 API 상태 = %d", listRes.StatusCode)
	}
}

func loginToken(t *testing.T, app *fiber.App, username, password string) string {
	t.Helper()
	res, err := app.Test(request(t, http.MethodPost, "/auth/login", types.LoginReq{Username: username, Password: password}, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("로그인 상태 = %d", res.StatusCode)
	}
	var token types.TokenRes
	if err := json.NewDecoder(res.Body).Decode(&token); err != nil {
		t.Fatal(err)
	}
	return token.Token
}

func request(t *testing.T, method, path string, body any, token string) *http.Request {
	t.Helper()
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequestWithContext(context.Background(), method, path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}
