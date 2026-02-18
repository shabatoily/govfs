package bootstrap

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/goccy/go-json"

	"github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/cloud/googledrive"
	"github.com/meteormin/govfs/config"
	"github.com/meteormin/govfs/drivers"
	"github.com/meteormin/govfs/server/middlewares"
	"github.com/meteormin/govfs/server/routes"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

const banner = `
   ____             __     
  / ___| _____   __/ _|___ 
 | |  _ / _ \ \ / / |_/ __|
 | |_| | (_) \ V /|  _\__ \
  \____|\___/ \_/ |_| |___/
`

func InitServer(fs vfs.VFS, cfg *config.ServerConfig) *fiber.App {
	// Set error handler
	cfg.ErrorHandler = middlewares.ErrorHandler
	// Set JSON encoder and decoder
	cfg.JSONEncoder = json.Marshal
	cfg.JSONDecoder = json.Unmarshal

	// Set fiber logger
	log.SetLevel(cfg.Logger.Level)
	fiberLogFile, err := os.Open(cfg.Logger.Path)
	if err != nil {
		log.SetOutput(os.Stdout)
	} else {
		w := io.MultiWriter(os.Stdout, fiberLogFile)
		log.SetOutput(w)
	}

	app := fiber.New(cfg.Config)

	middlewares.CommonMiddlewares(app, cfg)

	app.Get("/swagger/*", swaggo.New(swaggo.Config{
		Title: "Go VFS - Swagger UI",
	}))

	// web routes
	routes.Web(app, routes.DepsWeb{VFS: fs})

	app.Hooks().OnPreStartupMessage(func(preMsgData *fiber.PreStartupMessageData) error {
		preMsgData.BannerHeader = banner
		return nil
	})

	// on pre shutdown
	app.Hooks().OnPreShutdown(func() error {
		var err error
		if fs != nil {
			err = fs.Close()
		}
		if fiberLogFile != nil {
			err = errors.Join(err, fiberLogFile.Close())
		}
		return err
	})

	return app
}

func InitVFS(cfg *config.VfsConfig) (vfs.VFS, error) {
	vfsLogger, err := vfs.NewLogger(cfg.Logger)
	if err != nil {
		return nil, err
	}

	switch cfg.Driver.Type {
	case drivers.DriverTypeBadger:
		cfg.Driver.Badger.Logger = vfsLogger
	case drivers.DriverTypeLocalStorage:
		cfg.Driver.LocalStorage.Logger = vfsLogger
	}

	return drivers.New(cfg.Driver)
}

func InitCloud(ctx context.Context, cfg *config.CloudConfig) (cloud.Storage, error) {
	callbackUrl := "/google-drive/callback"
	lc := net.ListenConfig{}
	ln, _ := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	svr := fiber.New(fiber.Config{})
	svr.Get(callbackUrl, func(c fiber.Ctx) error {
		q := c.Queries()
		return c.Status(fiber.StatusOK).JSON(q)
	})
	go func() {
		if svrErr := svr.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}); svrErr != nil {
			panic(svrErr)
		}
	}()

	googleCloudCfg := cfg.GoogleDrive
	if googleCloudCfg.ClientID == "" || googleCloudCfg.ClientSecret == "" {
		return nil, errors.New("need to set env GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET")
	}

	oauthConfig := &oauth2.Config{
		ClientID:     googleCloudCfg.ClientID,
		ClientSecret: googleCloudCfg.ClientSecret,
		Scopes:       []string{drive.DriveScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  google.Endpoint.AuthURL,
			TokenURL: google.Endpoint.TokenURL,
		},
		RedirectURL: "http://localhost:" + strconv.Itoa(port) + callbackUrl,
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	tokenPath := filepath.Join(dir, ".govfs")
	if _, err = os.Stat(tokenPath); errors.Is(err, os.ErrNotExist) {
		err = os.Mkdir(tokenPath, vfs.DefaultDirMode)
		if err != nil {
			return nil, err
		}
	}

	client, err := googledrive.GetClient(tokenPath, oauthConfig)
	if err != nil {
		return nil, err
	}

	s, err := cloud.NewGoogleDriveStorage(ctx, client, googleCloudCfg.ParentFolderID)
	if err != nil {
		return nil, err
	}

	return s, nil
}
