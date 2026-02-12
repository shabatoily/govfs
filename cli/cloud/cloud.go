package cloud

import (
	"context"

	"github.com/meteormin/govfs/bootstrap"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/config"
)

func NewStorage(ctx context.Context, cfg *config.CloudConfig) (cloud.Storage, error) {
	return bootstrap.InitCloud(ctx, cfg)
}
