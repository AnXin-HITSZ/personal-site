package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/service"
)

// 和 accountService 同一个理由：把依赖收窄成接口，这一层的测试才只测 HTTP。
type adminArticleService interface {
	ListAdmin(ctx context.Context, query dto.AdminArticleListQuery) (dto.AdminArticleList, error)
	GetForEdit(ctx context.Context, id string) (dto.AdminArticleDetail, error)
	Create(ctx context.Context, input service.ArticleInput) (dto.AdminArticleDetail, error)
	Update(ctx context.Context, id string, input service.ArticleInput) (dto.AdminArticleDetail, error)
	Delete(ctx context.Context, id string) error
}

type AdminArticles struct {
	service adminArticleService
}

func NewAdminArticles(service adminArticleService) *AdminArticles {
	return &AdminArticles{service: service}
}

type articleRequest struct {
	Slug     string   `json:"slug"`
	Title    string   `json:"title"`
	Summary  string   `json:"summary"`
	Body     string   `json:"body"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Status   string   `json:"status"`
}

func (r articleRequest) input() service.ArticleInput {
	return service.ArticleInput{
		Slug:     r.Slug,
		Title:    r.Title,
		Summary:  r.Summary,
		Body:     r.Body,
		Category: r.Category,
		Tags:     r.Tags,
		Status:   r.Status,
	}
}

func (h *AdminArticles) List(c *gin.Context) {
	query, queryErr := parseAdminArticleListQuery(c)
	if queryErr != nil {
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument(queryErr.Field, queryErr.Message))
		return
	}

	result, err := h.service.ListAdmin(c.Request.Context(), query)
	if respondArticleError(c, err) {
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *AdminArticles) Get(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}

	detail, err := h.service.GetForEdit(c.Request.Context(), id)
	if respondArticleError(c, err) {
		return
	}

	c.JSON(http.StatusOK, detail)
}

func (h *AdminArticles) Create(c *gin.Context) {
	var request articleRequest
	if !bindJSONLimit(c, &request, maxArticleJSONBytes) {
		return
	}

	detail, err := h.service.Create(c.Request.Context(), request.input())
	if respondArticleError(c, err) {
		return
	}

	c.JSON(http.StatusCreated, detail)
}

func (h *AdminArticles) Update(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}

	var request articleRequest
	if !bindJSONLimit(c, &request, maxArticleJSONBytes) {
		return
	}

	detail, err := h.service.Update(c.Request.Context(), id, request.input())
	if respondArticleError(c, err) {
		return
	}

	c.JSON(http.StatusOK, detail)
}

func (h *AdminArticles) Delete(c *gin.Context) {
	id, ok := articleID(c)
	if !ok {
		return
	}

	if respondArticleError(c, h.service.Delete(c.Request.Context(), id)) {
		return
	}

	c.Status(http.StatusNoContent)
}

// 形状不对的 id 当作不存在，而不是格式错误：地址里那一段指的是某个资源，
// 回 400 等于白送探测者一个「你的 id 长错了」的信号。
func articleID(c *gin.Context) (string, bool) {
	id := c.Param("id")
	if !auth.IsShortID(id) {
		c.JSON(http.StatusNotFound, dto.NewNotFound("文章不存在"))
		return "", false
	}
	return id, true
}

// 把所有错误映射完，返回 true 表示响应已经发出去了。写成一个终结函数，
// 而不是让每个 handler 自己判——那样总有一个分支会被漏掉。
func respondArticleError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false

	case errors.Is(err, service.ErrArticleNotFound):
		c.JSON(http.StatusNotFound, dto.NewNotFound("文章不存在"))
	case errors.Is(err, service.ErrSlugTaken):
		c.JSON(http.StatusConflict, dto.NewConflict("slug", service.ErrSlugTaken.Error()))

	case errors.Is(err, service.ErrInvalidSlug):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("slug", service.ErrInvalidSlug.Error()))
	case errors.Is(err, service.ErrTitleRequired), errors.Is(err, service.ErrTitleTooLong):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("title", err.Error()))
	case errors.Is(err, service.ErrSummaryRequired), errors.Is(err, service.ErrSummaryTooLong):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("summary", err.Error()))
	case errors.Is(err, service.ErrBodyRequired), errors.Is(err, service.ErrBodyTooLong):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("body", err.Error()))
	case errors.Is(err, service.ErrInvalidCategory):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("category", service.ErrInvalidCategory.Error()))
	case errors.Is(err, service.ErrInvalidStatus):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("status", service.ErrInvalidStatus.Error()))
	case errors.Is(err, service.ErrTooManyTags), errors.Is(err, service.ErrTagTooLong):
		c.JSON(http.StatusBadRequest, dto.NewInvalidArgument("tags", err.Error()))

	default:
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, dto.NewInternalError())
	}
	return true
}

type adminArticleListParams struct {
	Page     int    `form:"page" binding:"min=1,max=1000000"`
	PageSize int    `form:"pageSize" binding:"min=1,max=100"`
	Keyword  string `form:"q"`
	Status   string `form:"status"`
	Category string `form:"category"`
}

func parseAdminArticleListQuery(c *gin.Context) (dto.AdminArticleListQuery, *queryError) {
	params := adminArticleListParams{Page: 1, PageSize: 20}
	if err := c.ShouldBindQuery(&params); err != nil {
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) && len(validationErrors) > 0 {
			return dto.AdminArticleListQuery{}, &queryError{validationErrors[0].Field(), "分页参数不合法"}
		}
		return dto.AdminArticleListQuery{}, &queryError{"page", "分页参数不合法"}
	}

	keyword := strings.TrimSpace(params.Keyword)
	if utf8.RuneCountInString(keyword) > 100 {
		return dto.AdminArticleListQuery{}, &queryError{"q", "搜索关键词不能超过 100 个字符"}
	}

	// 前端用 all 表示「不筛」，这里翻成空串——仓储那层空串就是不限。
	status := strings.TrimSpace(params.Status)
	switch status {
	case "", "all":
		status = ""
	case model.ArticleStatusDraft, model.ArticleStatusPublished:
	default:
		return dto.AdminArticleListQuery{}, &queryError{"status", "未知的状态"}
	}

	category := strings.TrimSpace(params.Category)
	if category == "all" {
		category = ""
	}
	if category != "" && !service.IsArticleCategory(category) {
		return dto.AdminArticleListQuery{}, &queryError{"category", "未知的分类"}
	}

	return dto.AdminArticleListQuery{
		Page:     params.Page,
		PageSize: params.PageSize,
		Keyword:  keyword,
		Status:   status,
		Category: category,
	}, nil
}
