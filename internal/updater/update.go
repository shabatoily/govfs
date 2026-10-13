// Package updater는 로컬 릴리즈 바이너리를 검증하고 교체합니다.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	downloadTimeout = 5 * time.Minute
	maxMetadataSize = 1 << 20
	privateDirMode  = 0o700
)

// Options는 현재 실행 파일과 업데이트 대상을 지정합니다.
type Options struct {
	Version    string
	Executable string
	Server     bool
	Check      bool
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

type updater struct {
	client      *http.Client
	apiURL      string
	downloadURL string
}

// Run은 로그인 없이 공식 GitHub 릴리즈를 확인하고 로컬 바이너리를 갱신합니다.
func Run(ctx context.Context, options Options, output io.Writer) error {
	return (&updater{
		client:      &http.Client{Timeout: downloadTimeout},
		apiURL:      "https://api.github.com/repos/shabatoily/govfs/releases/latest",
		downloadURL: "https://github.com/shabatoily/govfs/releases/download/",
	}).run(ctx, options, output)
}

func (u *updater) get(ctx context.Context, address string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "govfs-cli-updater")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", address, resp.StatusCode)
	}
	return resp, nil
}

func (u *updater) run(ctx context.Context, opts Options, output io.Writer) error {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return fmt.Errorf("self-update is unsupported on %s", runtime.GOOS)
	}
	latest, err := u.latest(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "CLI: %s; latest release: %s\n", opts.Version, latest.Tag)
	if opts.Check {
		return nil
	}
	if opts.Version == latest.Tag && !opts.Server {
		fmt.Fprintln(output, "Already up to date.")
		return nil
	}
	paths, names, err := targets(opts)
	if err != nil {
		return err
	}
	// 설치 디렉터리 잠금으로 업데이트 프로세스 간 경합을 방지합니다.
	lock := filepath.Join(filepath.Dir(paths[0]), ".govfs-update.lock")
	if err := os.Mkdir(lock, privateDirMode); err != nil {
		return fmt.Errorf("cannot lock installation directory; check permissions or existing lock (sudo may be required): %w", err)
	}
	defer os.Remove(lock)
	base := u.downloadURL + url.PathEscape(latest.Tag) + "/"
	assets, sums, err := u.manifest(ctx, latest, base)
	if err != nil {
		return err
	}
	staged := make([]replacement, 0, len(names))
	defer func() {
		for _, item := range staged {
			_ = os.Remove(item.temp)
		}
	}()
	for i, name := range names {
		asset := name + "-" + runtime.GOOS + "-" + runtime.GOARCH
		if !assets[asset] || sums[asset] == "" {
			return fmt.Errorf("release is missing binary or checksum for %s", asset)
		}
		temp, err := u.stage(ctx, base+asset, paths[i], sums[asset])
		if err != nil {
			return err
		}
		staged = append(staged, replacement{path: paths[i], temp: temp})
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := replace(staged); err != nil {
		return err
	}
	fmt.Fprintf(output, "Updated %s to %s.\n", strings.Join(names, " and "), latest.Tag)
	if opts.Server {
		fmt.Fprintln(output, "Restart the local server to use the new binary (govfs service restart for a registered service).")
	}
	return nil
}

func (u *updater) manifest(ctx context.Context, latest release, base string) (map[string]bool, map[string]string, error) {
	assets := make(map[string]bool)
	for _, asset := range latest.Assets {
		assets[asset.Name] = true
	}
	if !assets["checksums.txt"] {
		return nil, nil, errors.New("release has no checksums.txt; install a release with self-update support")
	}
	resp, err := u.get(ctx, base+"checksums.txt")
	if err != nil {
		return nil, nil, err
	}
	sums, err := parseChecksums(io.LimitReader(resp.Body, maxMetadataSize))
	_ = resp.Body.Close()
	if err != nil {
		return nil, nil, err
	}
	return assets, sums, nil
}

func (u *updater) latest(ctx context.Context) (release, error) {
	resp, err := u.get(ctx, u.apiURL)
	if err != nil {
		return release{}, err
	}
	var latest release
	err = json.NewDecoder(io.LimitReader(resp.Body, maxMetadataSize)).Decode(&latest)
	_ = resp.Body.Close()
	if err != nil {
		return release{}, err
	}
	if latest.Tag == "" {
		return release{}, errors.New("release has no version tag")
	}
	return latest, nil
}

func targets(opts Options) ([]string, []string, error) {
	path, err := filepath.EvalSymlinks(opts.Executable)
	if err != nil {
		return nil, nil, err
	}
	names := []string{"govfs-cli"}
	paths := []string{path}
	if opts.Server {
		serverPath, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(path), "govfs"))
		if err != nil {
			return nil, nil, fmt.Errorf("local server must exist beside the CLI: %w", err)
		}
		names = append(names, "govfs")
		paths = append(paths, serverPath)
	}
	return paths, names, nil
}

func parseChecksums(reader io.Reader) (map[string]string, error) {
	sums := make(map[string]string)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			return nil, errors.New("invalid checksum manifest")
		}
		hash, err := hex.DecodeString(fields[0])
		if err != nil || len(hash) != sha256.Size {
			return nil, errors.New("invalid SHA-256 checksum")
		}
		name := strings.TrimPrefix(fields[1], "*")
		if _, exists := sums[name]; exists {
			return nil, fmt.Errorf("duplicate checksum for %s", name)
		}
		sums[name] = hex.EncodeToString(hash)
	}
	return sums, scanner.Err()
}

func (u *updater) stage(ctx context.Context, address, path, checksum string) (_ string, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", path)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".govfs-download-*")
	if err != nil {
		return "", fmt.Errorf("cannot write installation directory; use sudo if required: %w", err)
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(file.Name())
		}
	}()
	resp, err := u.get(ctx, address)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	hash := sha256.New()
	// 비정상 응답으로 디스크가 가득 차지 않도록 다운로드 크기를 제한합니다.
	const maxSize = 512 << 20
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return "", err
	}
	if size > maxSize {
		return "", errors.New("release binary exceeds 512 MiB")
	}
	if hex.EncodeToString(hash.Sum(nil)) != checksum {
		return "", fmt.Errorf("checksum mismatch for %s", path)
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return file.Name(), nil
}

type replacement struct{ path, temp string }

// replace는 원본의 하드 링크를 보관해 교체 실패 시 이전 바이너리로 복구합니다.
func replace(items []replacement) error {
	backups := make([]string, 0, len(items))
	defer func() {
		for _, dir := range backups {
			if dir != "" {
				_ = os.RemoveAll(dir)
			}
		}
	}()
	for _, item := range items {
		dir, err := os.MkdirTemp(filepath.Dir(item.path), ".govfs-backup-*")
		if err != nil {
			return err
		}
		backups = append(backups, dir)
		if err := os.Link(item.path, filepath.Join(dir, "binary")); err != nil {
			return err
		}
	}
	for i, item := range items {
		if err := os.Rename(item.temp, item.path); err != nil {
			result := fmt.Errorf("replace %s: %w", item.path, err)
			for j := i - 1; j >= 0; j-- {
				backup := filepath.Join(backups[j], "binary")
				if err := os.Rename(backup, items[j].path); err != nil {
					result = errors.Join(result, fmt.Errorf("restore %s failed; original retained at %s: %w", items[j].path, backup, err))
					backups[j] = ""
				}
			}
			return result
		}
	}
	return nil
}
