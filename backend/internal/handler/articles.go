package handler

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/service"
)

var articleCategories = map[string]struct{}{
	"backend":  {},
	"frontend": {},
	"ai":       {},
	"notes":    {},
}

type ArticlesList struct {
	service *service.Article
}

func NewArticlesList(service *service.Article) *ArticlesList { return &ArticlesList{service: service} }

func (h *ArticlesList) List(c *gin.Context) {
	query, queryErr := parseArticleListQuery(c)
	if queryErr != nil {
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument(queryErr.Field, queryErr.Message))
		return
	}

	result, err := h.service.ListPublished(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusOK, result)
}

type articleListParams struct {
	Page     int    `form:"page" binding:"min=1,max=1000000"`
	PageSize int    `form:"pageSize" binding:"min=1,max=50"`
	Keyword  string `form:"q"`
	Category string `form:"category"`
}

type queryError struct {
	Field   string
	Message string
}

func parseArticleListQuery(c *gin.Context) (dto.ArticleListQuery, *queryError) {
	params := articleListParams{Page: 1, PageSize: 6}
	if err := c.ShouldBindQuery(&params); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) && len(validationErrors) > 0 {
			return dto.ArticleListQuery{}, &queryError{validationErrors[0].Field(), "分页参数不合法"}
		}
		return dto.ArticleListQuery{}, &queryError{"page", "分页参数不合法"}
	}

	keyword := strings.TrimSpace(params.Keyword)
	if utf8.RuneCountInString(keyword) > 100 {
		return dto.ArticleListQuery{}, &queryError{"q", "搜索关键词不能超过 100 个字符"}
	}

	if params.Category != "" {
		if _, ok := articleCategories[params.Category]; !ok {
			return dto.ArticleListQuery{}, &queryError{"category", "未知的分类"}
		}
	}

	return dto.ArticleListQuery{
		Page:     params.Page,
		PageSize: params.PageSize,
		Keyword:  keyword,
		Category: params.Category,
	}, nil
}
