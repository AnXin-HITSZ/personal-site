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
	"anxin-hitsz.com/backend/internal/service"
)

// 同时当读者那边的 categoryReader 和写作那边的 adminCategoryService 用：
// 一个假实现，两条路由都接它。
type fakeCategoryService struct {
	list      dto.AdminCategoryList
	published dto.CategoryList
	category  dto.AdminCategory
	err       error

	gotID     string
	gotName   string
	gotMoveTo string
	gotIDs    []string
}

func (f *fakeCategoryService) ListPublished(context.Context) (dto.CategoryList, error) {
	return f.published, f.err
}

func (f *fakeCategoryService) ListAdmin(context.Context) (dto.AdminCategoryList, error) {
	return f.list, f.err
}

func (f *fakeCategoryService) Create(_ context.Context, name string) (dto.AdminCategory, error) {
	f.gotName = name
	return f.category, f.err
}

func (f *fakeCategoryService) Rename(_ context.Context, id, name string) (dto.AdminCategory, error) {
	f.gotID = id
	f.gotName = name
	return f.category, f.err
}

func (f *fakeCategoryService) Reorder(_ context.Context, ids []string) error {
	f.gotIDs = ids
	return f.err
}

func (f *fakeCategoryService) Delete(_ context.Context, id, moveTo string) error {
	f.gotID = id
	f.gotMoveTo = moveTo
	return f.err
}

type categoryFixture struct {
	svc    *fakeCategoryService
	router *gin.Engine
}

func newCategoryFixture(role string) *categoryFixture {
	store := &fakeAdminSessionStore{role: role, sessions: map[string]model.Session{}}
	_ = store.CreateSession(context.Background(), &model.Session{
		TokenHash: auth.TokenHash(adminToken),
		UserID:    "u1",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	svc := &fakeCategoryService{}

	router := gin.New()
	router.Use(middleware.LogServerErrors(), middleware.Recovery())

	router.GET("/api/v1/categories", NewCategoriesList(svc).List)

	categories := NewAdminCategories(svc)
	admin := router.Group("/api/v1/admin",
		middleware.RequireAuth(middleware.NewSessionCookie(false), service.NewSession(store)),
		middleware.RequireRole(model.RoleAdmin))
	admin.GET("/categories", categories.List)
	admin.POST("/categories", categories.Create)
	admin.PUT("/categories/order", categories.Reorder)
	admin.PATCH("/categories/:id", categories.Rename)
	admin.DELETE("/categories/:id", categories.Delete)

	return &categoryFixture{svc: svc, router: router}
}

func (f *categoryFixture) request(method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://anxin-hitsz.com"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "axh_sid="+adminToken)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

// 读者那一行是公开的，问它不该要登录。
func (f *categoryFixture) anonymous(method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://anxin-hitsz.com"+path, nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field"`
}

func errorBodyOf(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body struct {
		Error errorBody `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是错误信封：%s", rec.Body.String())
	}
	return body.Error
}

func TestCategoryRoutesRejectAnonymous(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)

	rec := f.anonymous(http.MethodGet, "/api/v1/admin/categories")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

func TestCategoryRoutesRejectNonAdmin(t *testing.T) {
	f := newCategoryFixture(model.RoleMember)

	rec := f.request(http.MethodGet, "/api/v1/admin/categories", "")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通账号应 403，实际 %d：%s", rec.Code, rec.Body.String())
	}
}

// 分类 id 是定长的 8 位随机码，所以这一层只挡形状：旧的短代号、大写、带横线、
// 太长太短都当作「没有这个分类」，不是「你格式写错了」。
func TestAdminCategoryMalformedIDIsNotFound(t *testing.T) {
	for _, id := range []string{"backend", "Backend1", "backend-1", "0123456789abcdefg", "%E5%90%8E%E7%AB%AF"} {
		t.Run(id, func(t *testing.T) {
			f := newCategoryFixture(model.RoleAdmin)

			for _, call := range []struct {
				method string
				body   string
			}{
				{method: http.MethodPatch, body: `{"name":"服务端"}`},
				{method: http.MethodDelete},
			} {
				f.svc.gotID = ""
				rec := f.request(call.method, "/api/v1/admin/categories/"+id, call.body)

				if rec.Code != http.StatusNotFound {
					t.Fatalf("应 404，实际 %d：%s", rec.Code, rec.Body.String())
				}
				if body := errorBodyOf(t, rec); body.Code != dto.CodeNotFound {
					t.Errorf("应为 %s，实际 %s", dto.CodeNotFound, body.Code)
				}
				if f.svc.gotID != "" {
					t.Errorf("形状不对的 id 不该打到 service，实际收到了 %q", f.svc.gotID)
				}
			}
		})
	}
}

func TestAdminCategoryCreateReturns201(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)
	f.svc.category = dto.AdminCategory{ID: "abcd1234", Name: "读书笔记"}

	rec := f.request(http.MethodPost, "/api/v1/admin/categories", `{"name":"读书笔记"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("应 201，实际 %d：%s", rec.Code, rec.Body.String())
	}
	// 首尾空格由 service 收拾，这一层原样转手。
	if f.svc.gotName != "读书笔记" {
		t.Errorf("名字应原样传下去，实际 %q", f.svc.gotName)
	}
	if !strings.Contains(rec.Body.String(), `"id":"abcd1234"`) {
		t.Errorf("应回新建的那一个：%s", rec.Body.String())
	}
}

func TestAdminCategoryRenameReturns200(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)
	f.svc.category = dto.AdminCategory{ID: "abcd1234", Name: "服务端", ArticleCount: 4, DraftCount: 1}

	rec := f.request(http.MethodPatch, "/api/v1/admin/categories/abcd1234", `{"name":"服务端"}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if f.svc.gotID != "abcd1234" || f.svc.gotName != "服务端" {
		t.Errorf("应把 id 和名字都传下去，实际 %q / %q", f.svc.gotID, f.svc.gotName)
	}
	// 改名之后前端要就地更新那一行，所以回的是整个分类，不只是名字。
	if !strings.Contains(rec.Body.String(), `"articleCount":4`) {
		t.Errorf("应回两个计数：%s", rec.Body.String())
	}
}

// order 是 PUT 树上的静态路径，和 PATCH／DELETE 树上的 :id 各在各的树里——
// 这一条同时钉住「路由没被 :id 吃掉」和 204 的响应体是空的。
func TestAdminCategoryReorderReturns204(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)

	rec := f.request(http.MethodPut, "/api/v1/admin/categories/order", `{"ids":["efgh5678","abcd1234"]}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("应 204，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 不该有响应体，实际 %q", rec.Body.String())
	}
	if strings.Join(f.svc.gotIDs, ",") != "efgh5678,abcd1234" {
		t.Errorf("整份顺序应原样传下去，实际 %v", f.svc.gotIDs)
	}
}

func TestAdminCategoryDeleteReturns204(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)

	rec := f.request(http.MethodDelete, "/api/v1/admin/categories/abcd1234", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("应 204，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if f.svc.gotID != "abcd1234" || f.svc.gotMoveTo != "" {
		t.Errorf("没给去处时应传空串，实际 %q / %q", f.svc.gotID, f.svc.gotMoveTo)
	}
}

func TestAdminCategoryDeleteTrimsMoveTo(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)

	// 空串和「只有空格」在 service 那边是同一件事，所以在门口就统一掉。
	rec := f.request(http.MethodDelete, "/api/v1/admin/categories/abcd1234?moveTo=%20efgh5678%20", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("应 204，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if f.svc.gotMoveTo != "efgh5678" {
		t.Errorf("去处应去掉首尾空格，实际 %q", f.svc.gotMoveTo)
	}
}

func TestAdminCategoryMalformedBodyIs400(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)

	rec := f.request(http.MethodPost, "/api/v1/admin/categories", `{"name":`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if f.svc.gotName != "" {
		t.Errorf("请求体读不出来时不该打到 service，实际收到了 %q", f.svc.gotName)
	}
}

func TestAdminCategoryErrorMapping(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		want      int
		wantCode  string
		wantField string
	}{
		{name: "找不到", err: service.ErrCategoryNotFound, want: http.StatusNotFound, wantCode: dto.CodeNotFound},
		{name: "撞名", err: service.ErrCategoryNameTaken, want: http.StatusConflict, wantCode: dto.CodeConflict, wantField: "name"},
		// 「下面还有文章」指不到表单里的某一格，所以不带 field。
		{name: "下面还有文章", err: service.ErrCategoryInUse, want: http.StatusConflict, wantCode: dto.CodeConflict},
		{name: "名字为空", err: service.ErrCategoryNameRequired, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "name"},
		{name: "名字太长", err: service.ErrCategoryNameTooLong, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "name"},
		{name: "分类太多", err: service.ErrTooManyCategories, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "name"},
		{name: "挪到自己下面", err: service.ErrCategoryMoveToSelf, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "moveTo"},
		{name: "顺序对不上", err: service.ErrCategoryOrderMismatch, want: http.StatusBadRequest, wantCode: dto.CodeInvalidArgument, wantField: "ids"},
		// 数据库故障不能被当成用户填错了。
		{name: "数据库故障", err: errors.New("connection refused"), want: http.StatusInternalServerError, wantCode: dto.CodeInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCategoryFixture(model.RoleAdmin)
			f.svc.err = tc.err

			rec := f.request(http.MethodPost, "/api/v1/admin/categories", `{"name":"读书笔记"}`)

			if rec.Code != tc.want {
				t.Fatalf("应 %d，实际 %d：%s", tc.want, rec.Code, rec.Body.String())
			}
			body := errorBodyOf(t, rec)
			if body.Code != tc.wantCode {
				t.Errorf("应为 %s，实际 %s", tc.wantCode, body.Code)
			}
			if body.Field != tc.wantField {
				t.Errorf("field 应为 %q，实际 %q", tc.wantField, body.Field)
			}
		})
	}
}

func TestPublicCategoriesAreReadableWithoutASession(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)
	f.svc.published = dto.CategoryList{Items: []dto.CategoryRef{{ID: "abcd1234", Name: "后端开发"}}}

	rec := f.anonymous(http.MethodGet, "/api/v1/categories")

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []dto.CategoryRef `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%s", rec.Body.String())
	}
	if len(body.Items) != 1 || body.Items[0].Name != "后端开发" {
		t.Errorf("应回一个分类，实际 %+v", body.Items)
	}
	// 读者不该看见草稿数——那不是他要知道的事。
	if strings.Contains(rec.Body.String(), "articleCount") {
		t.Errorf("公开列表不该带写作那边的计数：%s", rec.Body.String())
	}
}

func TestPublicCategoriesFailureIs500(t *testing.T) {
	f := newCategoryFixture(model.RoleAdmin)
	f.svc.err = errors.New("connection refused")

	rec := f.anonymous(http.MethodGet, "/api/v1/categories")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("应 500，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if body := errorBodyOf(t, rec); body.Code != dto.CodeInternalError {
		t.Errorf("应为 %s，实际 %s", dto.CodeInternalError, body.Code)
	}
}
