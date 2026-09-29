package handler

import (
	"crypto/sha256"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/storage"
)

const (
	// 单张图的上限，必须小于 nginx 的 client_max_body_size。
	maxImageBytes = 4 << 20

	// 除图片外还要装下表单分隔符和字段名，给它们留一点余量。
	// 整个请求体封在这个数上，超出的部分连读都不会读。
	maxUploadBodyBytes = maxImageBytes + (64 << 10)

	imageTooLargeMessage = "图片不能超过 4 MB"

	storageNotConfiguredMessage = "图片存储未配置，无法上传"
)

type Uploads struct {
	storage storage.Store
}

func NewUploads(store storage.Store) *Uploads { return &Uploads{storage: store} }

func (h *Uploads) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBodyBytes)

	header, err := c.FormFile("file")
	if err != nil {
		if isBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, dto.NewPayloadTooLarge(imageTooLargeMessage))
			return
		}
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("file", "请在 file 字段里放一张图片"))
		return
	}

	file, err := header.Open()
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}
	defer file.Close()

	// 多读一个字节，用来分辨「正好到上限」和「超了」。
	data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
	if err != nil {
		if isBodyTooLarge(err) {
			c.JSON(http.StatusRequestEntityTooLarge, dto.NewPayloadTooLarge(imageTooLargeMessage))
			return
		}
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}
	if len(data) > maxImageBytes {
		c.JSON(http.StatusRequestEntityTooLarge, dto.NewPayloadTooLarge(imageTooLargeMessage))
		return
	}

	contentType, extension, ok := storage.SniffImage(data)
	if !ok {
		// 说清楚支持什么，比「文件类型不合法」有用。
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("file", "只支持 JPEG、PNG、GIF 和 WebP 图片"))
		return
	}

	// 不重新编码：省掉一个图像处理依赖，也省掉一次质量损失。代价是只能靠嗅探
	// 挡掉伪装的文件，挡不住「图片里藏着别的东西」——对这个 bucket 来说够了，
	// 它只被当作图片取用。
	object, err := h.storage.Put(c.Request.Context(), storage.ObjectKey(sha256.Sum256(data), extension), data, contentType)
	if err != nil {
		// 没配存储是这台机器缺一步，不是这次请求出了故障：分开报，界面才说得出
		// 原因。凭据不对、网络不通这类才是 500，细节由 c.Error 记进服务端日志。
		_ = c.Error(err)
		if errors.Is(err, storage.ErrNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, dto.NewServiceUnavailable(storageNotConfiguredMessage))
			return
		}
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusCreated, dto.NewUploadedImage(object.URL, object.Key, object.Bytes, object.ContentType))
}

// MaxBytesReader 在多层调用里可能被包过，所以认类型而不是判等。
func isBodyTooLarge(err error) bool {
	var tooLarge *http.MaxBytesError
	return errors.As(err, &tooLarge)
}
