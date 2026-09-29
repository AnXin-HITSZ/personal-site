package dto

import (
	"encoding/json"
	"testing"
	"time"

	"anxin-hitsz.com/backend/internal/model"
)

func TestArticleDetailMarshalsFlat(t *testing.T) {
	publishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	detail, ok := NewArticleDetail(model.Article{
		ID:             "a1",
		Slug:           "hello",
		Title:          "标题",
		Summary:        "摘要",
		Body:           "# 正文",
		Category:       "notes",
		Status:         "published",
		PublishedAt:    &publishedAt,
		ReadingMinutes: 3,
	})
	if !ok {
		t.Fatal("已发布且有发布时间，应能转换")
	}

	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"id", "slug", "title", "summary", "body", "category", "categoryName", "tags", "publishedAt", "readingMinutes"} {
		if _, present := fields[key]; !present {
			t.Errorf("缺少字段 %s：%s", key, raw)
		}
	}
	if _, nested := fields["ArticleSummary"]; nested {
		t.Errorf("ArticleSummary 被序列化成嵌套对象，详情响应必须平铺：%s", raw)
	}
	if string(fields["tags"]) != "[]" {
		t.Errorf("tags 为 nil 时应序列化为 []，实际为 %s", fields["tags"])
	}
}

// category 是筛选地址里那一段，categoryName 是页面上写出来的那几个字。名字不住在
// 文章表上，是仓储每次现取的；这一层只管把它搬过来，不要拿 id 去顶。
func TestCategoryAndItsNameAreTwoFields(t *testing.T) {
	publishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	summary, ok := NewArticleSummary(model.Article{
		ID: "a1", Status: "published", PublishedAt: &publishedAt, ReadingMinutes: 3,
		Category: "backend", CategoryName: "后端开发",
	})
	if !ok {
		t.Fatal("已发布且有发布时间，应能转换")
	}

	if summary.Category != "backend" || summary.CategoryName != "后端开发" {
		t.Errorf("两个字段应各是各的，实际 category=%q categoryName=%q", summary.Category, summary.CategoryName)
	}
}

func TestArticleDetailRejectsUnpublished(t *testing.T) {
	if _, ok := NewArticleDetail(model.Article{ID: "a2", Status: "draft"}); ok {
		t.Fatal("没有发布时间的文章不应能转换")
	}
}

func TestBodyRunesStaysInTheAdminFamily(t *testing.T) {
	publishedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	article := model.Article{ID: "a1", Status: "published", PublishedAt: &publishedAt, ReadingMinutes: 3, BodyRunes: 1200}

	fieldsOf := func(t *testing.T, v any) map[string]json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	admin := fieldsOf(t, NewAdminArticleSummary(article))
	if string(admin["bodyRunes"]) != "1200" {
		t.Errorf("后台响应应带上正文字数，实际 %s", admin["bodyRunes"])
	}

	public, ok := NewArticleSummary(article)
	if !ok {
		t.Fatal("已发布且有发布时间，应能转换")
	}
	// 读者看的是「几分钟」，字数只有写的人关心，不必进公开响应。
	if _, present := fieldsOf(t, public)["bodyRunes"]; present {
		t.Error("公开响应不该带正文字数")
	}
}
