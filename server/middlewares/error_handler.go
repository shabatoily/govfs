// Package middlewares는 Fiber 애플리케이션에서 사용하는 공통 및 커스텀 미들웨어를 제공합니다.
package middlewares

import (
	"errors"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"
	vfs "github.com/meteormin/govfs"
)

// ErrorHandler는 Fiber 애플리케이션에서 발생하는 에러를 캐치하여 공통 형식으로 응답하고 로깅합니다.
func ErrorHandler(c fiber.Ctx, err error) error {
	// 1. 기본 에러 핸들러 먼저 실행 (상태 코드 및 응답 설정)
	var chainingErr error
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		chainingErr = fiber.DefaultErrorHandler(c, fiberErr)
	} else {
		chainingErr = fiber.DefaultErrorHandler(c, handleVfsError(err))
	}

	// 2. 에러가 있을 경우에만 로깅
	if err != nil {
		log.Errorf("API Request Error method=%s endpoint=%s status=%d err=%s",
			c.Method(), c.OriginalURL(), c.Response().StatusCode(), err.Error())
	}

	return chainingErr
}

// handleVfsError는 VFS 드라이버에서 발생한 커스텀 에러를 HTTP 상태 코드에 맞는 Fiber 에러로 변환합니다.
func handleVfsError(err error) *fiber.Error {
	if errors.Is(err, vfs.ErrNotFound) {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if errors.Is(err, vfs.ErrAlreadyExists) {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	if errors.Is(err, vfs.ErrNotDir) {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if errors.Is(err, vfs.ErrNotFound) {
		return fiber.NewError(fiber.StatusFound, err.Error())
	}
	if errors.Is(err, vfs.ErrNotSupported) {
		return fiber.NewError(fiber.StatusNotImplemented, err.Error())
	}
	if errors.Is(err, vfs.ErrNotSupportedSeek) {
		return fiber.NewError(fiber.StatusNotImplemented, err.Error())
	}
	return fiber.NewError(fiber.StatusInternalServerError, err.Error())
}
