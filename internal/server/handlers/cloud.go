// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"net/url"

	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/internal/cloud"
	"github.com/meteormin/govfs/internal/cloud/googledrive"
	"github.com/meteormin/govfs/internal/types"
)

// CloudHandler는 외부 클라우드 저장소와의 상호작용을 처리하는 핸들러입니다.
type CloudHandler struct {
	storage cloud.Storage
}

// NewCloudHandler는 새로운 CloudHandler 인스턴스를 생성합니다.
func NewCloudHandler(storage cloud.Storage) *CloudHandler {
	return &CloudHandler{storage: storage}
}

// Prefix는 클라우드 라우트 그룹의 기본 접두사를 반환합니다.
func (h *CloudHandler) Prefix() string {
	return "/cloud"
}

// GoogleDriveCallbackURL은 구글 드라이브 인증 후 리디렉션 될 콜백 URL 경로를 반환합니다.
func (h *CloudHandler) GoogleDriveCallbackURL() string {
	return "/googledrive/callback"
}

// GoogleDriveAuthCodeURL은 Google Drive 인증을 위한 URL을 생성하여 반환합니다.
// @Summary      Google Drive 인증 URL
// @Description  Google Drive OAuth 인증을 위한 URL을 반환합니다.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      200  {object}  types.CloudAuthRes
// @Failure      400  {object}  string
// @Router       /cloud/googledrive/auth-code-url [get]
func (h *CloudHandler) GoogleDriveAuthCodeURL(c fiber.Ctx) error {
	googledriveAdaper, ok := h.storage.(*googledrive.Adapter)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "not a googledrive adapter")
	}

	redirectURL, err := url.JoinPath(c.BaseURL(), h.Prefix(), h.GoogleDriveCallbackURL())
	if err != nil {
		return err
	}

	return c.JSON(types.CloudAuthRes{
		URL: googledriveAdaper.AuthCodeURL(redirectURL, "state-token"),
	})
}

// GoogleDriveCallback은 Google Drive 인증 후 전달되는 콜백을 처리합니다.
// @Summary      Google Drive 콜백
// @Description  Google Drive 인증 콜백을 처리하고 토큰을 발급받습니다.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      200  {object}  string
// @Failure      400  {object}  string
// @Router       /cloud/googledrive/callback [get]
func (h *CloudHandler) GoogleDriveCallback(c fiber.Ctx) error {
	googledriveAdaper, ok := h.storage.(*googledrive.Adapter)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "not a googledrive adapter")
	}

	token, err := googledriveAdaper.IssueToken(c.Query("code"))
	if err != nil {
		return err
	}

	if err := googledriveAdaper.Init(token); err != nil {
		return err
	}

	return c.SendString("google drive authentication success! you can close this window.")
}

// IsAuthorized는 현재 클라우드 저장소가 올바르게 인증되어 있는지 확인합니다.
// @Summary      인증 상태 확인
// @Description  클라우드 저장소의 인증(인가) 여부를 확인합니다.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      204  {object}  nil
// @Failure      400  {object}  string
// @Router       /cloud/is-authorized [get]
func (h *CloudHandler) IsAuthorized(c fiber.Ctx) error {
	if _, ok := h.storage.(*googledrive.Adapter); !ok {
		return fiber.NewError(fiber.StatusBadRequest, "not a googledrive adapter")
	}
	if !h.storage.(*googledrive.Adapter).IsAuthorized() {
		return fiber.NewError(fiber.StatusUnauthorized, "not authorized")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// List는 클라우드 저장소 내의 파일 목록을 조회합니다.
// @Summary      파일 목록 조회
// @Description  클라우드 저장소의 특정 경로에 있는 파일 목록을 가져옵니다.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      200  {object}  types.CloudListRes
// @Failure      400  {object}  string
// @Router       /cloud/list [get]
func (h *CloudHandler) List(c fiber.Ctx) error {
	p := c.Query("path")
	files, err := h.storage.List(p)
	if err != nil {
		return err
	}

	return c.JSON(types.CloudListRes{
		Path:  p,
		Items: files,
	})
}

// Upload는 클라우드 저장소로 파일을 업로드합니다.
// @Summary      파일 업로드
// @Description  클라우드 저장소에 파일을 업로드합니다.
// @Tags         cloud
// @Accept       multipart/form-data
// @Produce      json
// @Success      204  {object}  nil
// @Failure      400  {object}  string
// @Router       /cloud/upload [post]
func (h *CloudHandler) Upload(c fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return err
	}

	m, err := file.Open()
	if err != nil {
		return err
	}
	defer m.Close()

	if err := h.storage.Upload(file.Filename, m); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// Download는 클라우드 저장소로부터 파일을 다운로드합니다.
// @Summary      파일 다운로드
// @Description  클라우드 저장소에서 지정된 파일을 다운로드합니다.
// @Tags         cloud
// @Accept       json
// @Produce      application/octet-stream
// @Success      200  {object}  io.ReadCloser
// @Failure      400  {object}  string
// @Router       /cloud/download [post]
func (h *CloudHandler) Download(c fiber.Ctx) error {
	r, err := h.storage.Download(c.Query("path"))
	if err != nil {
		return err
	}
	defer r.Close()

	return c.SendStream(r)
}

// Delete는 클라우드 저장소의 파일을 삭제합니다.
// @Summary      파일 삭제
// @Description  클라우드 저장소에서 지정된 파일을 삭제합니다.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      204  {object}  nil
// @Failure      400  {object}  string
// @Router       /cloud/delete [delete]
func (h *CloudHandler) Delete(c fiber.Ctx) error {
	if err := h.storage.Delete(c.Query("path")); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusNoContent)
}
