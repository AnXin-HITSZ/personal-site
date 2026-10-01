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

// 公开的读接口：一页文章、一篇详情。它们和后台那一族（admin_articles.go）分成两个文件，
// 是因为口径不同——从这里出去的必然已经发布，草稿碰都不该碰到。
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
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusOK, result)
}

// 按 id 取一篇已发布的。地址里那段 slug 不参与查询——改 slug 不作废任何链接，靠的就是这里。
type ArticleGet struct {
	service *service.Article
}

func NewArticleGet(service *service.Article) *ArticleGet { return &ArticleGet{service: service} }

func (h *ArticleGet) Get(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}

	detail, err := h.service.GetPublished(c.Request.Context(), id)
	if errors.Is(err, service.ErrArticleNotFound) {
		c.JSON(http.StatusNotFound, dto.NewNotFound("文章不存在"))
		return
	}
	if err != nil {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
		return
	}

	c.JSON(http.StatusOK, detail)
}

// 查询串的落点。binding 只管页和页大小；关键词与分类得先看内容（长度、字符集）再决定
// 收不收，那是下面的 parseArticleListQuery 在做的事。
type articleListParams struct {
	Page     int    `form:"page" binding:"min=1,max=1000000"`
	PageSize int    `form:"pageSize" binding:"min=1,max=50"`
	Keyword  string `form:"q"`
	Category string `form:"category"`
}

// 400 响应里那个 field 的来源：前端拿它对到出问题的那一格上。
type queryError struct {
	Field   string
	Message string
}

// 默认第一页、每页六篇：不带参数的请求也要有个说得通的结果，而不是一句 400。
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

	// 只查形状，不查它是不是真的存在：分类是一张表，每来一个筛选请求就为它跑一次
	// 查询不值当，而形状对、库里没有的那个筛出来本来就是空的。
	if params.Category != "" && !service.IsCategoryID(params.Category) {
		return dto.ArticleListQuery{}, &queryError{"category", "未知的分类"}
	}

	return dto.ArticleListQuery{
		Page:     params.Page,
		PageSize: params.PageSize,
		Keyword:  keyword,
		Category: params.Category,
	}, nil
}
