// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/internal/types"
	"github.com/meteormin/govfs/pkg/drivers/badger"
)

// BadgerHandler는 BadgerDB 전용 관리 기능을 처리하는 핸들러입니다.
type BadgerHandler struct {
	bvfs *badger.BadgerVFS
}

// NewBadgerHandler는 새로운 BadgerHandler 인스턴스를 생성합니다.
func NewBadgerHandler(bvfs *badger.BadgerVFS) *BadgerHandler {
	return &BadgerHandler{bvfs: bvfs}
}

// AllKeys는 데이터베이스에 저장된 모든 키(또는 특정 접두사를 가진 키) 목록을 반환합니다.
// @Summary      키 목록 조회
// @Description  데이터베이스 내의 모든 키를 조회합니다.
// @Tags         badger
// @Produce      json
// @Param        prefix    query     string  false  "prefix"
// @Success      200  {object}  types.BadgerKeyRes
// @Failure      500  {object}  fiber.Error
// @Router       /badger/keys [get]
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

// Stats는 데이터베이스의 키별 통계를 반환합니다.
// @Summary      DB 통계 조회
// @Description  DB 내의 메타, 블롭, 인덱스 별 통계를 조회합니다.
// @Tags         badger
// @Produce      json
// @Success      200  {object}  map[string]types.BadgerStatRes
// @Failure      500  {object}  fiber.Error
// @Router       /badger/stats [get]
func (h *BadgerHandler) Stats(ctx fiber.Ctx) error {
	stats, err := h.bvfs.Stats()
	if err != nil {
		return err
	}

	var totalCount int
	var totalSize int64
	for _, v := range stats {
		totalCount += v.Count
		totalSize += v.Size
	}

	return ctx.Status(fiber.StatusOK).JSON(types.BadgerStatRes{
		TotalCount: totalCount,
		TotalSize:  totalSize,
		PrefixBy:   stats,
	})
}

// Rotate 데이터 암호화 키를 교체합니다. (비동기 처리)
// @Summary      암호화 키 교체
// @Description  데이터베이스의 암호화 키를 교체(Rotate)합니다.
// @Tags         badger
// @Accept       json
// @Produce      json
// @Param        key    body    types.RotateKeyReq  true  "new key"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /badger/rotate [post]
func (h *BadgerHandler) Rotate(ctx fiber.Ctx) error {
	req := new(types.RotateKeyReq)
	if err := ctx.Bind().JSON(req); err != nil || req.Key == "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body or missing key")
	}
	if err := h.bvfs.Rotate([]byte(req.Key)); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	return ctx.SendStatus(fiber.StatusAccepted)
}
