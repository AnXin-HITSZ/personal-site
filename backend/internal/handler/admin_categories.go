package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/service"
)

type adminCategoryService interface {
	ListAdmin(ctx context.Context) (dto.AdminCategoryList, error)
	Create(ctx context.Context, name string) (dto.AdminCategory, error)
	Rename(ctx context.Context, id, name string) (dto.AdminCategory, error)
	Reorder(ctx context.Context, ids []string) error
	Delete(ctx context.Context, id, moveTo string) error
}

type AdminCategories struct {
	service adminCategoryService
}

func NewAdminCategories(service adminCategoryService) *AdminCategories {
	return &AdminCategories{service: service}
}

// 名字、还有一串最多二十个的 id。4KB 用不完。
type categoryNameRequest struct {
	Name string `json:"name"`
}

type categoryOrderRequest struct {
	IDs []string `json:"ids"`
}

func (h *AdminCategories) List(c *gin.Context) {
	result, err := h.service.ListAdmin(c.Request.Context())
	if respondCategoryError(c, err) {
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *AdminCategories) Create(c *gin.Context) {
	var request categoryNameRequest
	if !bindJSON(c, &request) {
		return
	}

	category, err := h.service.Create(c.Request.Context(), request.Name)
	if respondCategoryError(c, err) {
		return
	}

	c.JSON(http.StatusCreated, category)
}

// 只动名字那一格。顺序有它自己的接口——整份提交和单独改名是两回事。
func (h *AdminCategories) Rename(c *gin.Context) {
	id, ok := categoryID(c)
	if !ok {
		return
	}

	var request categoryNameRequest
	if !bindJSON(c, &request) {
		return
	}

	category, err := h.service.Rename(c.Request.Context(), id, request.Name)
	if respondCategoryError(c, err) {
		return
	}

	c.JSON(http.StatusOK, category)
}

// 上移、下移把整张表的新顺序一次发上来，所以这里收的是一整份，不是一个「和谁换」。
func (h *AdminCategories) Reorder(c *gin.Context) {
	var request categoryOrderRequest
	if !bindJSON(c, &request) {
		return
	}

	if respondCategoryError(c, h.service.Reorder(c.Request.Context(), request.IDs)) {
		return
	}

	c.Status(http.StatusNoContent)
}

// 分类下面还有文章时，moveTo 是这一批文章的去处。两者在服务端一个事务里完成：
// 先改文章的分类，再删分类——外键是 RESTRICT，顺序反了就删不掉。
func (h *AdminCategories) Delete(c *gin.Context) {
	id, ok := categoryID(c)
	if !ok {
		return
	}

	if respondCategoryError(c, h.service.Delete(c.Request.Context(), id, strings.TrimSpace(c.Query("moveTo")))) {
		return
	}

	c.Status(http.StatusNoContent)
}

// 和文章那边一样：形状不对的 id 当作不存在。分类 id 没有固定长度——内置那四个
// 是短代号，新生成的是 8 位随机码。
func categoryID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !service.IsCategoryID(id) {
		c.JSON(http.StatusNotFound, dto.NewNotFound("分类不存在"))
		return "", false
	}
	return id, true
}

func respondCategoryError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false

	case errors.Is(err, service.ErrCategoryNotFound):
		c.JSON(http.StatusNotFound, dto.NewNotFound("分类不存在"))

	// 撞名和「下面还有文章」都是请求本身没毛病、只是和已有的数据撞了。
	case errors.Is(err, service.ErrCategoryNameTaken):
		c.JSON(http.StatusConflict, dto.NewConflict("name", service.ErrCategoryNameTaken.Error()))
	case errors.Is(err, service.ErrCategoryInUse):
		c.JSON(http.StatusConflict, dto.NewConflict("", service.ErrCategoryInUse.Error()))

	case errors.Is(err, service.ErrCategoryNameRequired),
		errors.Is(err, service.ErrCategoryNameTooLong),
		errors.Is(err, service.ErrTooManyCategories):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("name", err.Error()))

	case errors.Is(err, service.ErrCategoryMoveToSelf):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("moveTo", service.ErrCategoryMoveToSelf.Error()))
	case errors.Is(err, service.ErrCategoryOrderMismatch):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("ids", service.ErrCategoryOrderMismatch.Error()))

	default:
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
	}
	return true
}
