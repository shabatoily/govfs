// Package handlers는 HTTP 요청을 처리하고 응답을 반환하는 핸들러를 제공합니다.
package handlers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/server/services"
	"github.com/shabatoily/govfs/internal/types"
)

const headerXClientID = "X-Client-ID"

// VfsHandler는 가상 파일 시스템(VFS) 관련 HTTP 요청을 처리하는 핸들러입니다.
type VfsHandler struct {
	srv    *services.VfsService
	broker *services.SSEBroker
	user   string
}

// Prefix는 VfsHandler가 담당하는 라우트의 URL 접두사를 반환합니다.
func (h *VfsHandler) Prefix() string {
	return h.srv.Prefix()
}

// List lists files and directories
//
//	@Summary      파일 및 디렉터리 목록 조회
//	@Description  지정된 경로의 하위 파일 및 디렉터리 목록을 가져옵니다.
//	@Tags         vfs
//	@Produce      json
//	@Param        q    query     string  false  "path" default(/)
//	@Param        viewType    query     string  false  "view type" Enums(list,tree)
//	@Success      200  {object}  types.VfsRes{payload=[]types.MetaRes}
//	@Success      200  {object}  types.VfsRes{payload=types.TreeNodeRes}
//	@Failure      400  {string}  string
//	@Failure      401  {string}  string
//	@Failure      404  {string}  string
//	@Failure      500  {string}  string
//	@Security     BearerAuth
//	@Router       /vfs [get]
//
// List는 파일 및 디렉토리 목록을 조회하여 반환합니다. (목록형 또는 트리형)
func (h *VfsHandler) List(ctx fiber.Ctx) error {
	q := ctx.Query("q", "/")
	t := ctx.Query("viewType", string(types.ViewTypeList))

	viewType := types.ViewType(t)
	switch viewType {
	case types.ViewTypeList:
		lsRes, err := h.srv.List(q)
		if err != nil {
			return err
		}
		res := types.VfsRes[[]types.MetaRes]{
			ViewType: viewType,
			Path:     q,
			Payload:  lsRes,
		}
		return ctx.Status(fiber.StatusOK).JSON(res)
	case types.ViewTypeTree:
		treeRes, err := h.srv.Tree(q)
		if err != nil {
			return err
		}
		res := types.VfsRes[*types.TreeNodeRes]{
			ViewType: viewType,
			Path:     q,
			Payload:  treeRes,
		}
		return ctx.Status(fiber.StatusOK).JSON(res)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "invalid viewType")
	}
}

// Search는 파일 및 디렉터리를 이름으로 검색합니다.
//
//	@Summary      파일 및 디렉터리 검색
//	@Description  전체 VFS에서 이름에 검색어가 포함된 파일 및 디렉터리를 검색합니다.
//	@Tags         vfs
//	@Produce      json
//	@Param        q    query     string  true  "검색어"
//	@Success      200  {array}   types.MetaRes
//	@Failure      400  {string}  string
//	@Failure      401  {string}  string
//	@Failure      500  {string}  string
//	@Security     BearerAuth
//	@Router       /vfs/search [get]
//
// Search는 파일 및 디렉터리 이름을 대소문자 구분 없이 검색합니다.
func (h *VfsHandler) Search(ctx fiber.Ctx) error {
	query := strings.TrimSpace(ctx.Query("q"))
	if query == "" {
		return fiber.NewError(fiber.StatusBadRequest, "missing search query")
	}

	results, err := h.srv.Search(query)
	if err != nil {
		return err
	}
	return ctx.Status(fiber.StatusOK).JSON(results)
}

// Read response file binary
//
//	@Summary      파일 읽기
//	@Description  파일의 바이너리 데이터를 다운로드(스트리밍)합니다.
//	@Tags         vfs
//	@Produce      octet-stream
//	@Param        id    path     string  true  "file id"
//	@Success      200  {file}    binary
//	@Success      206  {file}    binary
//	@Failure      400  {string}  string
//	@Failure      401  {string}  string
//	@Failure      404  {string}  string
//	@Failure      500  {string}  string
//	@Security     BearerAuth
//	@Router       /vfs/{id} [get]
//
// Read는 지정된 ID의 파일 바이너리 데이터를 스트리밍 응답으로 반환합니다. (Range 요청 지원)
func (h *VfsHandler) Read(ctx fiber.Ctx) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	file, err := h.srv.Read(parsedID)
	if err != nil {
		return err
	}
	defer file.Close()

	maxAge := "no-cache"
	cacheableMimeTypes := []string{"image", "video", "audio", "application/pdf", "application/octet-stream"}
	for _, mime := range cacheableMimeTypes {
		if strings.HasPrefix(file.Meta.MIME(), mime) {
			maxAge = "max-age=31536000"
		}
	}

	ctx.Set(fiber.HeaderCacheControl, maxAge)
	ctx.Set(fiber.HeaderContentType, file.Meta.MIME())
	ctx.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`inline; filename=%q`, file.Meta.Name))

	fileSize := file.Meta.Size

	// Range Request Handling
	ranges, err := ctx.Range(fileSize)
	if err == nil && len(ranges.Ranges) > 0 {
		// Only support single range for simple video streaming
		r := ranges.Ranges[0]
		start := r.Start
		end := r.End
		if end < 0 {
			end = fileSize - 1
		}

		if _, err := file.Seek(start, io.SeekStart); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to seek file")
		}

		contentLength := end - start + 1
		ctx.Status(fiber.StatusPartialContent)
		ctx.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		ctx.Set(fiber.HeaderContentLength, strconv.FormatInt(contentLength, 10))
		return ctx.SendStream(file, int(contentLength))
	}

	ctx.Set(fiber.HeaderContentLength, strconv.FormatInt(fileSize, 10))
	ctx.Status(fiber.StatusOK)
	return ctx.SendStream(file, int(fileSize))
}

// Stat stat a file or directory
// @Summary      메타데이터 조회
// @Description  파일 또는 디렉터리의 메타데이터를 조회합니다.
// @Tags         vfs
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Success      200  {object}  types.MetaRes
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      404  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/{id}/stat [get]
// Stat은 지정된 ID의 메타데이터 정보를 조회하여 반환합니다.
func (h *VfsHandler) Stat(ctx fiber.Ctx) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	meta, err := h.srv.Stat(parsedID)
	if err != nil {
		return err
	}

	return ctx.Status(fiber.StatusOK).JSON(meta)
}

// Create creates a new file or directory
// @Summary      생성 (파일/디렉터리)
// @Description  새로운 파일이나 디렉터리를 생성합니다.
// @Tags         vfs
// @Accept       multipart/form-data
// @Produce      plain
// @Param        isDir    formData     string  false  "is directory"
// @Param        name    formData     string  false  "name"
// @Param        file    formData  file    false   "file (isDir=false인 경우 필수)"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs [post]
// Create은 새로운 파일 또는 디렉토리를 생성합니다. (비동기 처리)
func (h *VfsHandler) Create(ctx fiber.Ctx) error {
	name := ctx.FormValue("name")
	isDir, convErr := fiber.Convert(ctx.FormValue("isDir", "false"), strconv.ParseBool)
	if convErr != nil {
		return fiber.NewError(fiber.StatusBadRequest, convErr.Error())
	}

	if isDir {
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "missing name field for directory creation")
		}

		h.asyncExecute(ctx.Get(headerXClientID), func() (types.SSEMeta, error) {
			meta, err := h.srv.Mkdir(name)
			return types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.create"}, err
		})
	} else {
		// 파일 생성 로직
		formFile, formErr := ctx.FormFile("file")
		if formErr != nil {
			return fiber.NewError(fiber.StatusBadRequest, formErr.Error())
		}

		// name이 제공되지 않으면 파일 이름 사용
		if name == "" {
			name = formFile.Filename
		}

		file, openErr := formFile.Open()
		if openErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, openErr.Error())
		}

		tempFile, tempErr := os.CreateTemp("", "govfs-upload-*")
		if tempErr != nil {
			_ = file.Close()
			return fiber.NewError(fiber.StatusInternalServerError, tempErr.Error())
		}
		tempPath := tempFile.Name()
		_, copyErr := io.Copy(tempFile, file)
		closeErr := file.Close()
		if copyErr != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return fiber.NewError(fiber.StatusInternalServerError, copyErr.Error())
		}
		if closeErr != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return fiber.NewError(fiber.StatusInternalServerError, closeErr.Error())
		}
		if _, seekErr := tempFile.Seek(0, io.SeekStart); seekErr != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
			return fiber.NewError(fiber.StatusInternalServerError, seekErr.Error())
		}

		h.asyncExecute(ctx.Get(headerXClientID), func() (types.SSEMeta, error) {
			defer func() {
				_ = tempFile.Close()
				_ = os.Remove(tempPath)
			}()
			meta, err := h.srv.Create(name, tempFile)
			return types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.create"}, err
		})
	}

	return ctx.SendStatus(fiber.StatusAccepted)
}

// Write write content to a file
// @Summary      파일 쓰기
// @Description  파일의 내용을 새로운 데이터로 덮어씁니다.
// @Tags         vfs
// @Accept       json
// @Produce      plain
// @Param        id    path     string  true  "file id"
// @Param        content    body    types.WriteReq  true  "content"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/{id} [put]
// Write는 파일 내용을 업데이트합니다. (비동기 처리)
func (h *VfsHandler) Write(ctx fiber.Ctx) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	req := new(types.WriteReq)
	err = ctx.Bind().JSON(req)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to parse request body")
	}

	// Deep Copy content for async processing
	content := req.Content
	h.asyncExecute(ctx.Get(headerXClientID), func() (types.SSEMeta, error) {
		meta, err := h.srv.Write(parsedID, bytes.NewBufferString(content))
		return types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.write"}, err
	})

	return ctx.SendStatus(fiber.StatusAccepted)
}

// Move rename or move a file or directory
// @Summary      이동 및 이름 변경
// @Description  파일 또는 디렉터리를 새로운 경로로 이동시키거나 이름을 변경합니다.
// @Tags         vfs
// @Accept       json
// @Produce      plain
// @Param        id    path     string  true  "file id"
// @Param        dst   body    types.DstReq  true  "destination"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/{id} [patch]
// Move는 파일 또는 디렉토리의 경로를 변경합니다. (비동기 처리)
func (h *VfsHandler) Move(ctx fiber.Ctx) error {
	return h.asyncModify(ctx, "vfs.move", h.srv.Move)
}

// Copy copy a file or directory
// @Summary      복사
// @Description  파일 또는 디렉터리를 새로운 경로로 복사합니다.
// @Tags         vfs
// @Accept       json
// @Produce      plain
// @Param        id    path     string  true  "file id"
// @Param        dst   body    types.DstReq  true  "destination"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/{id}/copy [post]
// Copy는 파일 또는 디렉토리를 지정된 경로로 복사합니다. (비동기 처리)
func (h *VfsHandler) Copy(ctx fiber.Ctx) error {
	return h.asyncModify(ctx, "vfs.copy", h.srv.Copy)
}

// Delete delete a file or directory
// @Summary      삭제
// @Description  지정된 파일 또는 디렉터리를 삭제합니다.
// @Tags         vfs
// @Produce      plain
// @Param        id    path     string  true  "file id"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/{id} [delete]
// Delete는 지정된 ID의 파일 또는 디렉토리를 삭제합니다. (비동기 처리)
func (h *VfsHandler) Delete(ctx fiber.Ctx) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	h.asyncExecute(ctx.Get(headerXClientID), func() (types.SSEMeta, error) {
		meta, statErr := h.srv.Stat(parsedID)
		if statErr != nil {
			return types.SSEMeta{ID: parsedID, Action: "vfs.delete"}, statErr
		}
		err := h.srv.Delete(parsedID)
		return types.SSEMeta{ID: parsedID, Path: meta.Path, Action: "vfs.delete"}, err
	})

	return ctx.SendStatus(fiber.StatusAccepted)
}

// WriteComments write comments to a file
// @Summary      주석(코멘트) 작성
// @Description  파일 또는 디렉터리에 부가적인 코멘트를 작성합니다.
// @Tags         vfs
// @Accept       json
// @Produce      plain
// @Param        id    		path     string  true  "file id"
// @Param        comment    body    types.WriteCommentReq  true  "comment"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/{id}/comments [patch]
// WriteComments는 파일 또는 디렉토리에 설명을 추가/수정합니다. (비동기 처리)
func (h *VfsHandler) WriteComments(ctx fiber.Ctx) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	req := new(types.WriteCommentReq)
	if err = ctx.Bind().JSON(req); err != nil || req.Comment == "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}

	// Deep Copy comment for async processing
	comment := req.Comment

	h.asyncExecute(ctx.Get(headerXClientID), func() (types.SSEMeta, error) {
		meta, err := h.srv.WriteComments(parsedID, comment)
		return types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.write-comments"}, err
	})

	return ctx.SendStatus(fiber.StatusAccepted)
}

// Backup backup all file
// @Summary      전체 백업
// @Description  전체 가상 파일 시스템의 데이터를 백업 파일(tar.gz)로 다운로드합니다.
// @Tags         vfs
// @Produce      octet-stream
// @Success      200  {file}	 string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/backup [post]
// Backup은 전체 VFS 데이터를 tar.gz 파일 형식으로 스트리밍 다운로드합니다.
func (h *VfsHandler) Backup(ctx fiber.Ctx) error {
	backupFilename := fmt.Sprintf("backup_%s.tar.gz", time.Now().Format("2006-01-02_15-04-05"))

	ctx.Set(fiber.HeaderContentDisposition, "attachment; filename="+backupFilename)

	r, w := io.Pipe()

	go func() {
		defer w.Close()
		err := h.srv.Backup(w)
		if err != nil {
			_ = w.CloseWithError(err)
		}
	}()

	return ctx.Status(fiber.StatusOK).SendStream(r)
}

// Restore restore all file
// @Summary      전체 복구
// @Description  업로드된 백업 파일을 통해 파일 시스템을 복구합니다.
// @Tags         vfs
// @Accept       multipart/form-data
// @Produce      plain
// @Param        file    formData  file    true    "file"
// @Success      200  {string}  string
// @Failure      400  {string}  string
// @Failure      401  {string}  string
// @Failure      500  {string}  string
// @Security     BearerAuth
// @Router       /vfs/restore [post]
// Restore는 업로드된 백업 파일로부터 VFS 데이터를 복구합니다.
func (h *VfsHandler) Restore(ctx fiber.Ctx) error {
	formFile, err := ctx.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "missing file field for restore")
	}

	file, err := formFile.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	if err := h.srv.Restore(file); err != nil {
		return err
	}

	return ctx.SendStatus(fiber.StatusOK)
}

// NewVfsHandler는 새로운 VfsHandler 인스턴스를 생성합니다.
func NewVfsHandler(srv *services.VfsService, broker *services.SSEBroker, user ...string) *VfsHandler {
	handler := &VfsHandler{srv: srv, broker: broker}
	if len(user) > 0 {
		handler.user = user[0]
	}
	return handler
}

// parseDstReq는 요청 구조체에서 대상 경로 정보(DstReq)를 파싱합니다.
func parseDstReq(ctx fiber.Ctx) (*types.DstReq, error) {
	req := new(types.DstReq)
	if err := ctx.Bind().JSON(req); err != nil || req.Name == "" {
		return nil, err
	}
	return req, nil
}

// ModifyFunc는 VFS 아이템을 수정(이동, 복사 등)하는 함수의 시그니처입니다.
type ModifyFunc func(id uuid.UUID, dst string) (types.MetaRes, error)

func (h *VfsHandler) asyncModify(ctx fiber.Ctx, action string, fn ModifyFunc) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	req, err := parseDstReq(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	h.asyncExecute(ctx.Get(headerXClientID), func() (types.SSEMeta, error) {
		m, err := fn(parsedID, req.Name)
		return types.SSEMeta{ID: m.ID, Path: m.Path, Action: action}, err
	})

	return ctx.SendStatus(fiber.StatusAccepted)
}

func (h *VfsHandler) asyncExecute(clientID string, do func() (types.SSEMeta, error)) {
	cid, notifyErr := uuid.Parse(clientID)
	go func() {
		meta, err := do()
		if notifyErr != nil {
			return
		}

		data := &types.SSEData{Timestamp: time.Now(), Status: err == nil, Meta: meta}
		if err != nil {
			data.Message = err.Error()
		}
		h.broker.Publish(h.user, cid, data, 0)
	}()
}
