package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/drivers/badger"
	"github.com/meteormin/govfs/server/types"
)

type BadgerHandler struct {
	bvfs *badger.BadgerVFS
}

func NewBadgerHandler(bvfs *badger.BadgerVFS) *BadgerHandler {
	return &BadgerHandler{bvfs: bvfs}
}

// AllKeys returns all file keys
// @Summary      AllKeys
// @Description  all keys
// @Tags         vfs
// @Produce      json
// @Param        prefix    query     string  false  "prefix"
// @Success      200  {object}  types.BadgerKeyRes
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/badger/keys [get]
func (h *BadgerHandler) AllKeys(ctx fiber.Ctx) error {
	var keys []string
	var err error

	prefix := ctx.Query("prefix", "")
	if prefix != "" {
		keys, err = h.bvfs.AllKeysByPrefix(prefix)
		if err != nil {
			return err
		}
	} else {
		keys, err = h.bvfs.AllKeys()
		if err != nil {
			return err
		}
	}

	return ctx.Status(fiber.StatusOK).JSON(types.BadgerKeyRes{
		Prefix: prefix,
		Keys:   keys,
	})
}
