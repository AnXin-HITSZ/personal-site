package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

// 和 accountService 同一个理由：把依赖收窄成接口，这一层的测试才只测 HTTP。
type categoryReader interface {
	ListPublished(ctx context.Context) (dto.CategoryList, error)
}

type CategoriesList struct {
	service categoryReader
}

func NewCategoriesList(service categoryReader) *CategoriesList {
	return &CategoriesList{service: service}
}

// 读者那一行筛选项。只列有已发布文章的分类，所以它是一个随写作变的列表，
// 不是一张配置——这也是它需要一个接口的原因。
func (h *CategoriesList) List(c *gin.Context) {
	result, err := h.service.ListPublished(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusOK, result)
}
