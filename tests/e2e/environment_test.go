package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pelletier/go-toml/v2"
	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/cli"
	"github.com/shabatoily/govfs/internal/types"
)

// environment는 드라이버별 서버·CLI·MCP 세션과 산출물 경로를 관리합니다.
type environment struct {
	ctx         context.Context
	dir         string
	bin         string
	base        string
	password    string
	secret      string
	token       string
	process     *exec.Cmd
	done        chan error
	session     *mcp.ClientSession
	fixtures    []fixture
	fixtureRoot string
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, name string, data []byte) {
	t.Helper()
	must(t, os.WriteFile(name, data, 0o600))
}

func command(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", args[0], err, data)
	}
	return data
}

func (e *environment) start(t *testing.T, driver string) {
	t.Helper()
	must(t, os.Mkdir(e.dir, 0o700))
	must(t, os.MkdirAll(filepath.Join(e.dir, "client", ".govfs"), 0o700))
	must(t, os.Mkdir(filepath.Join(e.dir, "logs"), 0o700))
	e.password = randomSecret(t)
	e.secret = randomSecret(t)
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	must(t, listener.Close())
	e.base = fmt.Sprintf("http://127.0.0.1:%d", port)
	config := fmt.Sprintf(`[server]
port = %d
[server.logger]
path = %q
accessLogPath = %q
[server.auth.admin]
username = "e2e-admin"
password = %q
[server.auth.jwt]
secret = %q
exp = "1h"
[server.fiber]
bodyLimit = 104857600
[server.webui]
enabled = true
[vfs.driver]
type = %q
[vfs.driver.badger]
path = %q
gcInterval = "5m"
gcDiscardRatio = 0.7
[vfs.driver.localstorage]
path = %q
[vfs.logger]
path = %q
`, port, filepath.Join(e.dir, "logs", "server.log"), filepath.Join(e.dir, "logs", "access.log"), e.password, e.secret, driver, filepath.Join(e.dir, "drives"), filepath.Join(e.dir, "drives"), filepath.Join(e.dir, "logs", "vfs.log"))
	write(t, filepath.Join(e.dir, "server.toml"), []byte(config))
	var cancel context.CancelFunc
	e.ctx, cancel = context.WithCancel(context.Background())
	t.Cleanup(func() {
		defer cancel()
		e.stop(t)
	})
	e.launch(t)
	var login types.TokenRes
	e.requestJSON(t, "POST", "/auth/login", map[string]string{"username": "e2e-admin", "password": e.password}, http.StatusOK, &login)
	e.token = login.Token
	sessionConfig, err := toml.Marshal(cli.UserConfig{ServerURL: e.base, Username: login.Username, TokenInfo: cli.TokenInfo{TokenRes: login}})
	must(t, err)
	write(t, filepath.Join(e.dir, "client", ".govfs", "config"), sessionConfig)
	e.connect(t)
}

func randomSecret(t *testing.T) string {
	t.Helper()
	var b [24]byte
	_, err := rand.Read(b[:])
	must(t, err)
	return hex.EncodeToString(b[:])
}

// 자식 프로세스에 기존 서버 설정과 프로젝트 .env를 전달하지 않습니다.
func isolatedEnv() []string {
	var env []string
	for _, v := range os.Environ() {
		key, _, _ := strings.Cut(v, "=")
		if !strings.HasPrefix(key, "SERVER_") && !strings.HasPrefix(key, "VFS_") {
			env = append(env, v)
		}
	}
	return env
}

func (e *environment) launch(t *testing.T) {
	t.Helper()
	log, err := os.OpenFile(filepath.Join(e.dir, "logs", "process.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	must(t, err)
	defer log.Close()
	e.process = exec.CommandContext(e.ctx, filepath.Join(e.bin, "server"), "--config", filepath.Join(e.dir, "server.toml"))
	e.process.Dir = e.dir
	e.process.Env = isolatedEnv()
	e.process.Stdout = log
	e.process.Stderr = log
	if err := e.process.Start(); err != nil {
		e.process = nil
		t.Fatal(err)
	}
	e.done = make(chan error, 1)
	go func() { e.done <- e.process.Wait() }()
	client := http.Client{Timeout: time.Second}
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		req, err := http.NewRequestWithContext(e.ctx, http.MethodGet, e.base+"/healthz", http.NoBody)
		must(t, err)
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	e.stop(t)
	t.Fatal("server did not become ready; see logs/process.log")
}

func (e *environment) connect(t *testing.T) {
	t.Helper()
	log, err := os.OpenFile(filepath.Join(e.dir, "logs", "mcp.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	must(t, err)
	defer log.Close()
	cmd := exec.CommandContext(e.ctx, filepath.Join(e.bin, "cli"), "--config", filepath.Join(e.dir, "client"), "mcp")
	cmd.Dir = e.dir
	cmd.Env = isolatedEnv()
	cmd.Stderr = log
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	e.session, err = mcp.NewClient(&mcp.Implementation{Name: "govfs-e2e", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	must(t, err)
}

func (e *environment) stop(t *testing.T) {
	t.Helper()
	if e.session != nil {
		_ = e.session.Close()
		e.session = nil
	}
	if e.process == nil {
		return
	}
	_ = e.process.Process.Signal(os.Interrupt)
	select {
	case <-e.done:
	case <-time.After(5 * time.Second):
		_ = e.process.Process.Kill()
		<-e.done
		t.Error("server required forced termination")
	}
	e.process = nil
}

func (e *environment) request(t *testing.T, method, path string, data any, headers http.Header) (int, http.Header, []byte) {
	t.Helper()
	var body io.Reader
	if data != nil {
		b, err := json.Marshal(data)
		must(t, err)
		body = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, e.base+path, body)
	must(t, err)
	if headers != nil {
		req.Header = headers.Clone()
	}
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	must(t, err)
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	must(t, err)
	return res.StatusCode, res.Header, b
}

func (e *environment) requestJSON(t *testing.T, method, path string, data any, want int, out any) {
	t.Helper()
	status, _, b := e.request(t, method, path, data, nil)
	if status != want {
		t.Fatalf("%s %s: status %d, want %d", method, path, status, want)
	}
	if out != nil {
		must(t, json.Unmarshal(b, out))
	}
}

func (e *environment) callTool(t *testing.T, name string, args, out any, wantError bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	result, err := e.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	// 연결 단절은 입력 검증 성공으로 간주하지 않습니다.
	must(t, err)
	if result.IsError != wantError {
		detail, marshalErr := json.Marshal(result.Content)
		must(t, marshalErr)
		t.Fatalf("%s isError=%v, want %v: %s", name, result.IsError, wantError, detail)
	}
	if wantError {
		return
	}
	if out != nil {
		b, err := json.Marshal(result.StructuredContent)
		must(t, err)
		if result.StructuredContent == nil {
			for _, c := range result.Content {
				if text, ok := c.(*mcp.TextContent); ok {
					b = []byte(text.Text)
					break
				}
			}
		}
		must(t, json.Unmarshal(b, out))
	}
}

func (e *environment) upload(t *testing.T, path string, data []byte) vfs.Meta {
	t.Helper()
	var meta vfs.Meta
	e.callTool(t, "vfs_upload", map[string]string{"path": path, "content_base64": base64.StdEncoding.EncodeToString(data)}, &meta, false)
	return meta
}

func poll(t *testing.T, fn func() bool) {
	t.Helper()
	end := time.Now().Add(30 * time.Second)
	for time.Now().Before(end) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("state did not converge within 30 seconds")
}

func (e *environment) cli(t *testing.T, args ...string) {
	t.Helper()
	command(t, e.dir, append([]string{filepath.Join(e.bin, "cli"), "--config", filepath.Join(e.dir, "client")}, args...)...)
}
