package middlewares

import (
	"testing"

	"github.com/gofiber/fiber/v3"
	vfs "github.com/shabatoily/govfs"
	"github.com/stretchr/testify/assert"
)

func TestHandleVfsErrorInvalidPath(t *testing.T) {
	err := handleVfsError(vfs.ErrInvalidPath)
	assert.Equal(t, fiber.StatusBadRequest, err.Code)
}
