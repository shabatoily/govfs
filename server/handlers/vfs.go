package handlers

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/meteormin/govfs/server/services"
	"github.com/meteormin/govfs/server/types"
)

type VfsHandler struct {
	srv    *services.VfsService
	broker *services.SSEBroker
}

func (h *VfsHandler) Prefix() string {
	return h.srv.Prefix()
}

// List lists files and directories
//
//	@Summary      List files and directories
//	@Description  get files and directories
//	@Tags         vfs
//	@Produce      json
//	@Param        q    query     string  false  "name search by q"
//	@Param        viewType    query     string  false  "view type" Enums(list,tree)
//	@Success      200  {object}  types.VfsRes{payload=[]types.MetaRes}
//	@Success      200  {object}  types.VfsRes{payload=types.TreeNodeRes}
//	@Failure      400  {object}  fiber.Error
//	@Failure      404  {object}  fiber.Error
//	@Failure      500  {object}  fiber.Error
//	@Router       /vfs [get]
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

// Read response file binary
//
//	@Summary      Read file
//	@Description  read file
//	@Tags         vfs
//	@Produce      octet-stream
//	@Param        id    path     string  true  "file id"
//	@Success      200  {file}	 string
//	@Failure      400  {object}  fiber.Error
//	@Failure      404  {object}  fiber.Error
//	@Failure      500  {object}  fiber.Error
//	@Router       /vfs/:id [get]
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
// @Summary      Stat file
// @Description  stat file
// @Tags         vfs
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Success      200  {object}  types.MetaRes
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/:id/stat [get]
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
// @Summary      Create file or directory
// @Description  create file or directory
// @Tags         vfs
// @Accept       multipart/form-data
// @Produce      json
// @Param        isDir    formData     string  false  "is directory"
// @Param        name    formData     string  false  "name"
// @Param        file    formData  file    true    "file"
// @Success      201  {object}  types.MetaRes
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs [post]
func (h *VfsHandler) Create(ctx fiber.Ctx) error {
	var meta types.MetaRes
	var err error

	name := ctx.FormValue("name")
	isDir, convErr := fiber.Convert(ctx.FormValue("isDir", "false"), strconv.ParseBool)
	if convErr != nil {
		return fiber.NewError(fiber.StatusBadRequest, convErr.Error())
	}

	if isDir {
		if name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "missing name field for directory creation")
		}

		meta, err = h.srv.Mkdir(name)
		if err != nil {
			return err
		}
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
		defer func() {
			_ = file.Close()
		}()

		meta, err = h.srv.Create(name, file)
	}

	clientID := ctx.Get("X-Client-ID")
	cid, parseErr := uuid.Parse(clientID)
	if parseErr == nil {
		eventMeta := types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.create"}
		if err != nil {
			h.broker.Error(cid, &types.SSEData{Timestamp: time.Now(), Status: false, Meta: eventMeta, Message: err.Error()}, time.Second*3)
			return err
		}

		h.broker.Publish(cid, &types.SSEData{Timestamp: time.Now(), Status: true, Meta: eventMeta}, time.Second*3)
	}

	return ctx.Status(fiber.StatusCreated).JSON(meta)
}

// Write write content to a file
// @Summary      Write content to a file
// @Description  write content to a file
// @Tags         vfs
// @Accept       json
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Param        content    body    types.WriteReq  true  "content"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/:id [put]
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
	clientID := ctx.Get("X-Client-ID")
	cid, parseErr := uuid.Parse(clientID)
	if parseErr == nil {
		h.broker.AsyncExcute(cid, func() (types.SSEMeta, error) {
			meta, err := h.srv.Write(parsedID, bytes.NewBufferString(content))
			return types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.write"}, err
		})
	}

	return ctx.SendStatus(fiber.StatusAccepted)
}

// Move rename or move a file or directory
// @Summary      Move file or directory
// @Description  move file or directory
// @Tags         vfs
// @Accept       json
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Param        dst   body    types.DstReq  true  "destination"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/:id [patch]
func (h *VfsHandler) Move(ctx fiber.Ctx) error {
	return h.asyncModify(ctx, h.srv.Move)
}

// Copy copy a file or directory
// @Summary      Copy file or directory
// @Description  copy file or directory
// @Tags         vfs
// @Accept       json
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Param        dst   body    types.DstReq  true  "destination"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/:id/copy [post]
func (h *VfsHandler) Copy(ctx fiber.Ctx) error {
	return h.asyncModify(ctx, h.srv.Copy)
}

// Delete delete a file or directory
// @Summary      Delete file or directory
// @Description  delete file or directory
// @Tags         vfs
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/:id [delete]
func (h *VfsHandler) Delete(ctx fiber.Ctx) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	clientID := ctx.Get("X-Client-ID")
	cid, parseErr := uuid.Parse(clientID)
	if parseErr == nil {
		h.broker.AsyncExcute(cid, func() (types.SSEMeta, error) {
			err := h.srv.Delete(parsedID)
			return types.SSEMeta{ID: parsedID, Path: "", Action: "vfs.delete"}, err
		})
	}

	return ctx.SendStatus(fiber.StatusAccepted)
}

// WriteComments write comments to a file
// @Summary      Write comments to a file
// @Description  write comments to a file
// @Tags         vfs
// @Accept       json
// @Produce      json
// @Param        id    path     string  true  "file id"
// @Param        comment    query    string  true  "comment"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      404  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/:id/comments [patch]
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

	clientID := ctx.Get("X-Client-ID")
	cid, parseErr := uuid.Parse(clientID)
	if parseErr == nil {
		h.broker.AsyncExcute(cid, func() (types.SSEMeta, error) {
			meta, err := h.srv.WriteComments(parsedID, comment)
			return types.SSEMeta{ID: meta.ID, Path: meta.Path, Action: "vfs.write-comments"}, err
		})
	}

	return ctx.SendStatus(fiber.StatusAccepted)
}

// Backup backup all file
// @Summary      Backup all file
// @Description  backup all file
// @Tags         vfs
// @Produce      octet-stream
// @Success      200  {file}	 string
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/backup [post]
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
// @Summary      Restore all file
// @Description  restore all file
// @Tags         vfs
// @Accept       multipart/form-data
// @Produce      json
// @Param        file    formData  file    true    "file"
// @Success      200  {string}  string
// @Failure      400  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/restore [post]
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

// Rotate rotate encryption key
// @Summary      Rotate encryption key
// @Description  rotate encryption key
// @Tags         vfs
// @Accept       json
// @Produce      json
// @Param        key    body     string  true  "new key"
// @Success      202  {string}  string "Accepted"
// @Failure      400  {object}  fiber.Error
// @Failure      500  {object}  fiber.Error
// @Router       /vfs/rotate [post]
func (h *VfsHandler) Rotate(ctx fiber.Ctx) error {
	type rotateReq struct {
		Key string `json:"key"`
	}
	req := new(rotateReq)
	if err := ctx.Bind().JSON(req); err != nil || req.Key == "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body or missing key")
	}

	// Deep Copy key for async processing
	newKey := req.Key
	clientID := ctx.Get("X-Client-ID")
	cid, parseErr := uuid.Parse(clientID)
	if parseErr == nil {
		h.broker.AsyncExcute(cid, func() (types.SSEMeta, error) {
			return types.SSEMeta{Action: "vfs.rotate"}, h.srv.Rotate(newKey)
		})
	}

	return ctx.SendStatus(fiber.StatusAccepted)
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
func (h *VfsHandler) AllKeys(ctx fiber.Ctx) error {
	var keys []string
	var err error

	prefix := ctx.Query("prefix", "")
	if prefix != "" {
		keys, err = h.srv.AllKeysByPrefix(prefix)
		if err != nil {
			return err
		}
	} else {
		keys, err = h.srv.AllKeys()
		if err != nil {
			return err
		}
	}

	return ctx.Status(fiber.StatusOK).JSON(types.BadgerKeyRes{
		Prefix: prefix,
		Keys:   keys,
	})
}

func NewVfsHandler(srv *services.VfsService, broker *services.SSEBroker) *VfsHandler {
	return &VfsHandler{srv: srv, broker: broker}
}

func parseDstReq(ctx fiber.Ctx) (*types.DstReq, error) {
	req := new(types.DstReq)
	if err := ctx.Bind().JSON(req); err != nil || req.Name == "" {
		return nil, err
	}
	return req, nil
}

type ModifyFunc func(id uuid.UUID, dst string) (types.MetaRes, error)

func (h *VfsHandler) asyncModify(ctx fiber.Ctx, fn ModifyFunc) error {
	parsedID, err := fiber.Convert(ctx.Params("id"), uuid.Parse)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	req, err := parseDstReq(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	clientID := ctx.Get("X-Client-ID")
	cid, parseErr := uuid.Parse(clientID)
	if parseErr == nil {
		h.broker.AsyncExcute(cid, func() (types.SSEMeta, error) {
			m, err := fn(parsedID, req.Name)
			return types.SSEMeta{ID: m.ID, Path: m.Path, Action: "vfs.move"}, err
		})
	}

	return ctx.SendStatus(fiber.StatusAccepted)
}
