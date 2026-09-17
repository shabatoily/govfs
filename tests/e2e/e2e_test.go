package e2e

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	enabled    = flag.Bool("e2e", false, "Run isolated live-server tests")
	output     = flag.String("e2e-dir", "", "New artifact directory (default: temporary directory)")
	fixtureDir = flag.String("e2e-fixtures", "", "Reuse fixtures and manifest.json from an earlier run directory")
	drivers    = flag.String("e2e-drivers", "badger,localstorage", "Drivers to test sequentially")
)

type fixture struct {
	Path     string `json:"path"`
	Size     int    `json:"size"`
	SHA256   string `json:"sha256"`
	MIME     string `json:"mime"`
	Renderer string `json:"renderer"`
}

// TestE2E은 명시적으로 활성화한 경우에만 서버와 생성기를 실행합니다.
func TestE2E(t *testing.T) {
	if !*enabled {
		t.Skip("use -e2e to run live-server tests")
	}
	driverList := strings.Split(*drivers, ",")
	for _, d := range driverList {
		if d != "badger" && d != "localstorage" {
			t.Fatalf("invalid driver: %s", d)
		}
	}
	repo, err := filepath.Abs("../..")
	must(t, err)
	var dir string
	if *output == "" {
		dir, err = os.MkdirTemp("", "govfs-e2e-")
	} else {
		dir, err = filepath.Abs(*output)
		must(t, err)
		err = os.Mkdir(dir, 0o700)
	}
	must(t, err)
	t.Logf("Artifacts: %s", dir)
	must(t, os.Mkdir(filepath.Join(dir, "bin"), 0o700))
	write(t, filepath.Join(dir, "revision.txt"), command(t, repo, "git", "rev-parse", "HEAD"))
	write(t, filepath.Join(dir, "working-tree.txt"), command(t, repo, "git", "status", "--short"))
	write(t, filepath.Join(dir, "web-build.log"), command(t, repo, "yarn", "--cwd", "webui", "build"))
	bin := filepath.Join(dir, "bin")
	command(t, repo, "go", "build", "-o", filepath.Join(bin, "server"), "./cmd/govfs")
	command(t, repo, "go", "build", "-o", filepath.Join(bin, "cli"), "./cmd/govfs-cli")
	source := dir
	if *fixtureDir != "" {
		source, err = filepath.Abs(*fixtureDir)
		must(t, err)
	} else {
		f := filepath.Join(dir, "fixtures")
		command(t, repo, "go", "run", "./tools/gen/image", "-count", "2", "-width", "320", "-height", "240", "-out", f)
		command(t, repo, "go", "run", "./tools/gen/video", "-count", "1", "-duration", "2", "-width", "320", "-height", "240", "-out", f)
		command(t, repo, "go", "run", "./tools/gen/text", "-count", "2", "-bytes", "4096", "-out", f)
		command(t, repo, "python3", "tests/e2e/fixtures.py", dir)
	}
	manifest, err := os.ReadFile(filepath.Join(source, "manifest.json"))
	must(t, err)
	var fixtures []fixture
	must(t, json.Unmarshal(manifest, &fixtures))
	if len(fixtures) == 0 {
		t.Fatal("empty fixture manifest")
	}
	for _, f := range fixtures {
		if !filepath.IsLocal(f.Path) {
			t.Fatalf("invalid fixture path: %s", f.Path)
		}
		b, err := os.ReadFile(filepath.Join(source, "fixtures", f.Path))
		must(t, err)
		if len(b) != f.Size || fmt.Sprintf("%x", sha256.Sum256(b)) != f.SHA256 {
			t.Fatalf("fixture changed: %s", f.Path)
		}
	}
	write(t, filepath.Join(dir, "manifest.json"), manifest)
	write(t, filepath.Join(dir, "fixture-source.txt"), []byte(source+"\n"))
	for _, driver := range driverList {
		t.Run(driver, func(t *testing.T) {
			e := &environment{
				dir:         filepath.Join(dir, driver),
				bin:         bin,
				fixtures:    fixtures,
				fixtureRoot: filepath.Join(source, "fixtures"),
			}
			e.start(t, driver)
			e.runScenarios(t, driver)
		})
	}
}
