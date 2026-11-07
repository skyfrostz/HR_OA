package api

import (
	"errors"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// UploadFile godoc
// @Tags OA Contracts
// @Summary HR 管理员上传私有合同附件
// @Security ApiKeyAuth
// @Accept mpfd
// @Produce json
// @Param id path int true "合同ID"
// @Param file formData file true "PDF、JPEG 或 PNG，最多 10 MiB"
// @Success 201 {object} response.Response{data=model.FileView}
// @Failure 400,401,403,404,409,413,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/contracts/{id}/files [post]
func (a *API) UploadFile(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	header, cleanup, err := singleFileUpload(c, service.MaxFileSize)
	defer cleanup()
	if err != nil {
		fail(c, err)
		return
	}
	file, err := header.Open()
	if err != nil {
		fail(c, err)
		return
	}
	defer file.Close()
	result, err := a.service.SaveFile(c.Request.Context(), Actor(c), id, header.Filename, file)
	if err != nil {
		fail(c, err)
		return
	}
	succeed(c, http.StatusCreated, result)
}

// DownloadFile godoc
// @Tags OA Contracts
// @Summary 下载权限范围内的私有合同附件
// @Security ApiKeyAuth
// @Produce application/pdf,image/jpeg,image/png
// @Param id path int true "附件ID"
// @Success 200 {file} file
// @Header 200 {string} Content-Disposition "attachment"
// @Header 200 {string} Cache-Control "no-store"
// @Header 200 {string} X-Content-Type-Options "nosniff"
// @Failure 400,401,403,404,500 {object} response.Response{data=errorData}
// @Router /api/oa/v1/files/{id}/download [get]
func (a *API) DownloadFile(c *gin.Context) {
	id, ok := resourceID(c)
	if !ok {
		return
	}
	view, file, err := a.service.OpenFile(c.Request.Context(), Actor(c), id)
	if err != nil {
		fail(c, err)
		return
	}
	defer file.Close()
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": view.OriginalName})
	if disposition == "" {
		fail(c, errors.New("invalid stored attachment name"))
		return
	}
	c.DataFromReader(http.StatusOK, view.Size, view.ContentType, file, map[string]string{"Content-Disposition": disposition})
}

func singleFileUpload(c *gin.Context, maxFileSize int64) (*multipart.FileHeader, func(), error) {
	cleanup := func() {}
	maxRequest := maxFileSize + 64*1024
	if c.Request.ContentLength > maxRequest {
		return nil, cleanup, service.ErrTooLarge
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequest)
	err := c.Request.ParseMultipartForm(1024 * 1024)
	form := c.Request.MultipartForm
	if form != nil {
		cleanup = func() { _ = form.RemoveAll() }
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, cleanup, err
		}
		return nil, cleanup, service.ErrInvalid
	}
	if form == nil {
		return nil, cleanup, service.ErrInvalid
	}
	files := form.File["file"]
	if len(form.File) != 1 || len(files) != 1 || len(form.Value) != 0 {
		return nil, cleanup, service.ErrInvalid
	}
	if files[0].Size > maxFileSize {
		return nil, cleanup, service.ErrTooLarge
	}
	return files[0], cleanup, nil
}
