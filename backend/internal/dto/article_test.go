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

	for _, key := range []string{"id", "slug", "title", "summary", "body", "category", "tags", "publishedAt", "readingMinutes"} {
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

func TestArticleDetailRejectsUnpublished(t *testing.T) {
	if _, ok := NewArticleDetail(model.Article{ID: "a2", Status: "draft"}); ok {
		t.Fatal("没有发布时间的文章不应能转换")
	}
}
