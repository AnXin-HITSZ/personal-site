package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"anxin-hitsz.com/backend/internal/auth"
	"anxin-hitsz.com/backend/internal/dto"
	"anxin-hitsz.com/backend/internal/middleware"
	"anxin-hitsz.com/backend/internal/model"
	"anxin-hitsz.com/backend/internal/repository"
	"anxin-hitsz.com/backend/internal/service"
	"anxin-hitsz.com/backend/internal/storage"
)

// 会话表按令牌哈希索引，所以固定一个令牌、算出它的哈希存进去，
// 请求带上原始令牌就能通过中间件。
const adminToken = "admin-session-token"

type fakeAdminArticleService struct {
	detail   dto.AdminArticleDetail
	list     dto.AdminArticleList
	err      error
	gotQuery dto.AdminArticleListQuery
	gotInput service.ArticleInput
	gotID    string
	deleted  string
}

func (f *fakeAdminArticleService) ListAdmin(_ context.Context, query dto.AdminArticleListQuery) (dto.AdminArticleList, error) {
	f.gotQuery = query
	return f.list, f.err
}

func (f *fakeAdminArticleService) GetForEdit(_ context.Context, id string) (dto.AdminArticleDetail, error) {
	f.gotID = id
	return f.detail, f.err
}

func (f *fakeAdminArticleService) Create(_ context.Context, input service.ArticleInput) (dto.AdminArticleDetail, error) {
	f.gotInput = input
	return f.detail, f.err
}

func (f *fakeAdminArticleService) Update(_ context.Context, id string, input service.ArticleInput) (dto.AdminArticleDetail, error) {
	f.gotID = id
	f.gotInput = input
	return f.detail, f.err
}

func (f *fakeAdminArticleService) Delete(_ context.Context, id string) error {
	f.deleted = id
	return f.err
}

// 只认得 adminToken 这一个会话，角色由测试指定。
type fakeAdminSessionStore struct {
	role     string
	sessions map[string]model.Session
}

func (f *fakeAdminSessionStore) CreateSession(_ context.Context, session *model.Session) error {
	f.sessions[session.TokenHash] = *session
	return nil
}

func (f *fakeAdminSessionStore) GetSessionByTokenHash(_ context.Context, tokenHash string) (*model.Session, error) {
	session, ok := f.sessions[tokenHash]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return &session, nil
}

func (f *fakeAdminSessionStore) GetUserByID(_ context.Context, id string) (*model.User, error) {
	if id != "u1" {
		return nil, repository.ErrNotFound
	}
	return &model.User{
		ID:     "u1",
		Email:  "owner@example.com",
		Role:   f.role,
		Status: model.UserStatusActive,
	}, nil
}

func (f *fakeAdminSessionStore) DeleteSessionByTokenHash(_ context.Context, tokenHash string) error {
	delete(f.sessions, tokenHash)
	return nil
}

type adminFixture struct {
	svc    *fakeAdminArticleService
	router *gin.Engine
}

func newAdminFixture(role string) *adminFixture {
	store := &fakeAdminSessionStore{role: role, sessions: map[string]model.Session{}}
	_ = store.CreateSession(context.Background(), &model.Session{
		TokenHash: auth.TokenHash(adminToken),
		UserID:    "u1",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	cookies := middleware.NewSessionCookie(false)
	svc := &fakeAdminArticleService{}
	adminArticles := NewAdminArticles(svc)
	uploads := NewUploads(storage.UnconfiguredStore{})

	router := gin.New()
	router.Use(middleware.LogServerErrors(), middleware.Recovery())

	admin := router.Group("/api/v1/admin",
		middleware.RequireAuth(cookies, service.NewSession(store)),
		middleware.RequireRole(model.RoleAdmin))
	admin.GET("/articles", adminArticles.List)
	admin.POST("/articles", adminArticles.Create)
	admin.GET("/articles/:id", adminArticles.Get)
	admin.PUT("/articles/:id", adminArticles.Update)
	admin.DELETE("/articles/:id", adminArticles.Delete)
	admin.POST("/uploads", uploads.Create)

	return &adminFixture{svc: svc, router: router}
}

func (f *adminFixture) request(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://anxin-hitsz.com"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "axh_sid="+adminToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func errorCodeOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code  string `json:"code"`
			Field string `json:"field"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是错误信封：%s", rec.Body.String())
	}
	return body.Error.Code
}

const validArticleBody = `{"slug":"building-my-blog","title":"标题","summary":"摘要","body":"正文","category":"backend","tags":["go"],"status":"draft"}`

func TestAdminRoutesRejectNonAdmin(t *testing.T) {
	f := newAdminFixture(model.RoleMember)

	rec := f.request(http.MethodGet, "/api/v1/admin/articles", "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通账号应 403，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCodeOf(t, rec); code != dto.CodeForbidden {
		t.Errorf("应为 %s，实际 %s", dto.CodeForbidden, code)
	}
}

func TestAdminRoutesRejectAnonymous(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)

	req := httptest.NewRequest(http.MethodGet, "http://anxin-hitsz.com/api/v1/admin/articles", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，实际 %d", rec.Code)
	}
}

func TestAdminMalformedIDIsNotFound(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)

	// 形状不对、太长、带大写，都是「没有这一篇」，不是「你格式写错了」。
	for _, id := range []string{"short", "a7k2m9pqr", "A7K2M9PQ", strings.Repeat("a", 32)} {
		t.Run(id, func(t *testing.T) {
			rec := f.request(http.MethodGet, "/api/v1/admin/articles/"+id, "")

			if rec.Code != http.StatusNotFound {
				t.Fatalf("应 404，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if f.svc.gotID != "" {
				t.Errorf("形状不对的 id 不该打到 service，实际收到了 %q", f.svc.gotID)
			}
		})
	}
}

func TestAdminGetPassesIDThrough(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)

	rec := f.request(http.MethodGet, "/api/v1/admin/articles/a7k2m9pq", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if f.svc.gotID != "a7k2m9pq" {
		t.Errorf("id 应原样传下去，实际 %q", f.svc.gotID)
	}
}

func TestAdminCreateReturns201(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)

	rec := f.request(http.MethodPost, "/api/v1/admin/articles", validArticleBody)

	if rec.Code != http.StatusCreated {
		t.Fatalf("应 201，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if f.svc.gotInput.Slug != "building-my-blog" || f.svc.gotInput.Category != "backend" {
		t.Errorf("请求体没有完整落到 service：%+v", f.svc.gotInput)
	}
	if len(f.svc.gotInput.Tags) != 1 || f.svc.gotInput.Tags[0] != "go" {
		t.Errorf("标签没有落到 service：%+v", f.svc.gotInput.Tags)
	}
}

func TestAdminDeleteReturns204(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)

	rec := f.request(http.MethodDelete, "/api/v1/admin/articles/a7k2m9pq", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("应 204，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 不该有响应体，实际 %q", rec.Body.String())
	}
	if f.svc.deleted != "a7k2m9pq" {
		t.Errorf("应删掉 a7k2m9pq，实际 %q", f.svc.deleted)
	}
}

func TestAdminErrorMapping(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		want      int
		wantCode  string
		wantField string
	}{
		{name: "找不到", err: service.ErrArticleNotFound, want: http.StatusNotFound, wantCode: dto.CodeNotFound},
		{name: "slug 撞车", err: service.ErrSlugTaken, want: http.StatusConflict, wantCode: dto.CodeConflict, wantField: "slug"},
		{name: "slug 不合法", err: service.ErrInvalidSlug, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "slug"},
		{name: "标题为空", err: service.ErrTitleRequired, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "title"},
		{name: "标题太长", err: service.ErrTitleTooLong, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "title"},
		{name: "摘要为空", err: service.ErrSummaryRequired, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "summary"},
		{name: "正文为空", err: service.ErrBodyRequired, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "body"},
		{name: "分类未知", err: service.ErrInvalidCategory, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "category"},
		{name: "状态未知", err: service.ErrInvalidStatus, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "status"},
		{name: "标签太多", err: service.ErrTooManyTags, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "tags"},
		// 数据库故障不能被当成用户填错了。
		{name: "数据库故障", err: errors.New("connection refused"), want: http.StatusInternalServerError, wantCode: dto.CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAdminFixture(model.RoleAdmin)
			f.svc.err = tc.err

			rec := f.request(http.MethodPost, "/api/v1/admin/articles", validArticleBody)

			if rec.Code != tc.want {
				t.Fatalf("应 %d，实际 %d：%s", tc.want, rec.Code, rec.Body.String())
			}
			if code := errorCodeOf(t, rec); code != tc.wantCode {
				t.Errorf("应为 %s，实际 %s", tc.wantCode, code)
			}
			if tc.wantField != "" {
				var body struct {
					Error struct {
						Field string `json:"field"`
					} `json:"error"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				if body.Error.Field != tc.wantField {
					t.Errorf("field 应为 %q，实际 %q", tc.wantField, body.Error.Field)
				}
			}
		})
	}
}

func TestAdminOversizedBodyIs413(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)

	body := `{"body":"` + strings.Repeat("正", maxArticleJSONBytes) + `"}`
	rec := f.request(http.MethodPost, "/api/v1/admin/articles", body)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("应 413，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code := errorCodeOf(t, rec); code != dto.CodePayloadTooLarge {
		t.Errorf("应为 %s，实际 %s", dto.CodePayloadTooLarge, code)
	}
	if f.svc.gotInput.Body != "" {
		t.Error("超限的请求不该打到 service")
	}
}

func TestAdminListQueryParsing(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		want      dto.AdminArticleListQuery
		wantCode  int
		wantField string
	}{
		{
			name:  "默认值",
			query: "",
			want:  dto.AdminArticleListQuery{Page: 1, PageSize: 20},
		},
		{
			name:  "all 表示不筛",
			query: "?status=all&category=all",
			want:  dto.AdminArticleListQuery{Page: 1, PageSize: 20},
		},
		{
			name:  "按草稿筛",
			query: "?status=draft&category=ai&page=2&pageSize=5&q=%20go%20",
			want:  dto.AdminArticleListQuery{Page: 2, PageSize: 5, Status: "draft", Category: "ai", Keyword: "go"},
		},
		{name: "状态未知", query: "?status=archived", wantCode: http.StatusBadRequest, wantField: "status"},
		// 分类是一张表，这一层只认形状：写得不像 id 的挡掉，库里恰好没有的那一个
		// 筛出来就是空的，不在这一层报错。
		{name: "分类形状不对", query: "?category=Life", wantCode: http.StatusBadRequest, wantField: "category"},
		{name: "库里没有的分类", query: "?category=life", want: dto.AdminArticleListQuery{Page: 1, PageSize: 20, Category: "life"}},
		{name: "页码为零", query: "?page=0", wantCode: http.StatusBadRequest},
		{name: "每页太多", query: "?pageSize=101", wantCode: http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAdminFixture(model.RoleAdmin)

			rec := f.request(http.MethodGet, "/api/v1/admin/articles"+tc.query, "")

			if tc.wantCode != 0 {
				if rec.Code != tc.wantCode {
					t.Fatalf("应 %d，实际 %d：%s", tc.wantCode, rec.Code, rec.Body.String())
				}
				if tc.wantField != "" {
					var body struct {
						Error struct {
							Field string `json:"field"`
						} `json:"error"`
					}
					_ = json.Unmarshal(rec.Body.Bytes(), &body)
					if body.Error.Field != tc.wantField {
						t.Errorf("field 应为 %q，实际 %q", tc.wantField, body.Error.Field)
					}
				}
				return
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("应 200，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if f.svc.gotQuery != tc.want {
				t.Errorf("解析结果应为 %+v，实际 %+v", tc.want, f.svc.gotQuery)
			}
		})
	}
}

// 后台的 DTO 里 publishedAt 可空——草稿就是没有。用 JSON 断言而不是比结构体，
// 是为了连「序列化出来到底是不是 null」一起测掉。
func TestAdminDraftSerializesNullPublishedAt(t *testing.T) {
	f := newAdminFixture(model.RoleAdmin)
	f.svc.detail = dto.AdminArticleDetail{
		AdminArticleSummary: dto.AdminArticleSummary{
			ID:     "a7k2m9pq",
			Slug:   "draft-post",
			Status: model.ArticleStatusDraft,
			Tags:   []string{},
		},
		Body: "正文",
	}

	rec := f.request(http.MethodGet, "/api/v1/admin/articles/a7k2m9pq", "")

	var body struct {
		PublishedAt *time.Time `json:"publishedAt"`
		Tags        []string   `json:"tags"`
		Body        string     `json:"body"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%s", rec.Body.String())
	}
	if body.PublishedAt != nil {
		t.Errorf("草稿的 publishedAt 应为 null，实际 %v", body.PublishedAt)
	}
	if !strings.Contains(rec.Body.String(), `"publishedAt":null`) {
		t.Errorf("publishedAt 应显式序列化成 null：%s", rec.Body.String())
	}
	if body.Body != "正文" {
		t.Errorf("编辑页要拿到正文，实际 %q", body.Body)
	}
}
