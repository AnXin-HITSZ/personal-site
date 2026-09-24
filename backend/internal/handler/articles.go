package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type ArticlesList struct{}

func NewArticlesList() *ArticlesList { return &ArticlesList{} }

func (h *ArticlesList) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": []any{},
		"pagination": gin.H{
			"page":       1,
			"pageSize":   6,
			"total":      0,
			"totalPages": 0,
		},
	})
}
