package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/dto"
)

type ArticlesList struct{}

func NewArticlesList() *ArticlesList { return &ArticlesList{} }

func (h *ArticlesList) List(c *gin.Context) {
	c.JSON(http.StatusOK, dto.ArticleList{
		Items: []dto.ArticleSummary{},
		Pagination: dto.Pagination{
			Page:       1,
			PageSize:   6,
			Total:      0,
			TotalPages: 0,
		},
	})
}
