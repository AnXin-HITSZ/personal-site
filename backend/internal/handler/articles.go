package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/service"
)

type ArticlesList struct {
	service *service.Article
}

func NewArticlesList(service *service.Article) *ArticlesList { return &ArticlesList{service: service} }

func (h *ArticlesList) List(c *gin.Context) {
	type ListQuery struct {
		Page     int `form:"page" binding:"min=1"`
		PageSize int `form:"pageSize" binding:"min=1,max=1000000"`
	}
	req := ListQuery{
		Page:     1,
		PageSize: 6,
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		var verr validator.ValidationErrors
		if errors.As(err, &verr) && len(verr) > 0 {
			c.JSON(http.StatusBadRequest, dto.NewInvalidArgument(verr[0].Field(), "分页参数不合法"))
			return
		}
	}
	result, err := h.service.ListPublished(c.Request.Context(), req.Page, req.PageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}
	c.JSON(http.StatusOK, result)
}
