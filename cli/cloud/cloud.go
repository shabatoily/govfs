package cloud

import (
	"context"

	"github.com/meteormin/go-vfs/bootstrap"
	"github.com/meteormin/go-vfs/cloud"
	"github.com/meteormin/go-vfs/config"
)

func NewStorage(ctx context.Context, cfg *config.CloudConfig) (cloud.Storage, error) {
	return bootstrap.InitCloud(ctx, cfg)
}
