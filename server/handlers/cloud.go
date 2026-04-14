// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"github.com/gofiber/fiber/v3"
	"github.com/meteormin/govfs/cloud"
	"github.com/meteormin/govfs/cloud/googledrive"
	"github.com/meteormin/govfs/server/types"
)

const GoogleAuthCodeCallbackURL = "/cloud/googledrive/callback"

// CloudHandler는 외부 클라우드 저장소와의 상호작용을 처리하는 핸들러입니다.
type CloudHandler struct {
	storage cloud.Storage
}

// NewCloudHandler는 새로운 CloudHandler 인스턴스를 생성합니다.
func NewCloudHandler(storage cloud.Storage) *CloudHandler {
	return &CloudHandler{storage: storage}
}

// GoogleDriveAuthCodeURL returns the authentication URL for Google Drive.
// @Summary      Google Drive Auth Code URL
// @Description  Returns the authentication URL for Google Drive.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      200  {object}  types.CloudAuthResponse
// @Failure      400  {object}  string
// @Router       /cloud/googledrive/auth-code-url [get]
// GoogleDriveAuthCodeURL은 Google Drive 인증을 위한 URL을 생성하여 반환합니다.
func (h *CloudHandler) GoogleDriveAuthCodeURL(c fiber.Ctx) error {
	googledriveAdaper, ok := h.storage.(*googledrive.Adapter)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "not a googledrive adapter")
	}

	return c.JSON(types.CloudAuthResponse{
		URL: googledriveAdaper.AuthCodeURL(GoogleAuthCodeCallbackURL, "state-token"),
	})
}

// GoogleDriveCallback handles the callback from the Google Drive authentication.
// @Summary      Google Drive Callback
// @Description  Handles the callback from the Google Drive authentication.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      200  {object}  string
// @Failure      400  {object}  string
// @Router       /cloud/googledrive/callback [get]
// GoogleDriveCallback은 Google Drive 인증 후 전달되는 콜백을 처리합니다.
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

// List lists the files in the cloud storage.
// @Summary      List files
// @Description  Lists the files in the cloud storage.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      200  {object}  types.CloudListResponse
// @Failure      400  {object}  string
// @Router       /cloud/list [get]
// List는 클라우드 저장소 내의 파일 목록을 조회합니다.
func (h *CloudHandler) List(c fiber.Ctx) error {
	p := c.Query("path")
	files, err := h.storage.List(p)
	if err != nil {
		return err
	}

	return c.JSON(types.CloudListResponse{
		Path:  p,
		Items: files,
	})
}

// Upload uploads a file to the cloud storage.
// @Summary      Upload file
// @Description  Uploads a file to the cloud storage.
// @Tags         cloud
// @Accept       multipart/form-data
// @Produce      json
// @Success      204  {object}  nil
// @Failure      400  {object}  string
// @Router       /cloud/upload [post]
// Upload는 클라우드 저장소로 파일을 업로드합니다.
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

// Download downloads a file from the cloud storage.
// @Summary      Download file
// @Description  Downloads a file from the cloud storage.
// @Tags         cloud
// @Accept       json
// @Produce      application/octet-stream
// @Success      200  {object}  io.ReadCloser
// @Failure      400  {object}  string
// @Router       /cloud/download [post]
// Download는 클라우드 저장소로부터 파일을 다운로드합니다.
func (h *CloudHandler) Download(c fiber.Ctx) error {
	r, err := h.storage.Download(c.Query("path"))
	if err != nil {
		return err
	}
	defer r.Close()

	return c.SendStream(r)
}

// Delete deletes a file from the cloud storage.
// @Summary      Delete file
// @Description  Deletes a file from the cloud storage.
// @Tags         cloud
// @Accept       json
// @Produce      json
// @Success      204  {object}  nil
// @Failure      400  {object}  string
// @Router       /cloud/delete [delete]
// Delete는 클라우드 저장소의 파일을 삭제합니다.
func (h *CloudHandler) Delete(c fiber.Ctx) error {
	if err := h.storage.Delete(c.Query("path")); err != nil {
		return err
	}

	return c.SendStatus(fiber.StatusNoContent)
}
