package main

import (
	"encoding/json"
	"github.com/example/dbx-plugin-rss/rss"
	dbxpluginsdk "github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk"
)

func (p *Plugin) libraryRPC(method string, params json.RawMessage) (any, *dbxpluginsdk.PluginError) {
	var input struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		FeedID     string `json:"feedId"`
		ArticleID  string `json:"articleId"`
		CategoryID string `json:"categoryId"`
		Favorite   bool   `json:"favorite"`
		Favorites  bool   `json:"favorites"`
		Notes      bool   `json:"notes"`
		Search     string `json:"search"`
		Limit      int    `json:"limit"`
		Offset     int    `json:"offset"`
	}
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, invalidParams("invalid library request")
		}
	}
	switch method {
	case "rss/library/index":
		return map[string]any{"items": p.service.LibraryIndex(), "categories": p.service.ListCategories()}, nil
	case "rss/library/list":
		limit := input.Limit
		if limit <= 0 || limit > 200 {
			limit = 40
		}
		items, total := p.service.ListLibrary(rss.LibraryQuery{Favorites: input.Favorites, Notes: input.Notes, CategoryID: input.CategoryID, Search: input.Search, Limit: limit, Offset: input.Offset})
		return map[string]any{"items": items, "total": total}, nil
	case "rss/library/article":
		a, err := p.service.GetArticle(input.FeedID, input.ArticleID)
		if err != nil {
			return nil, rssError(err)
		}
		return a, nil
	case "rss/library/saveCategory":
		g, err := p.service.SaveCategory(input.ID, input.Name)
		if err != nil {
			return nil, rssError(err)
		}
		return g, nil
	case "rss/library/deleteCategory":
		if err := p.service.DeleteCategory(input.ID); err != nil {
			return nil, rssError(err)
		}
	case "rss/library/setFavorite":
		if err := p.service.SetFavorite(input.FeedID, input.ArticleID, input.Favorite, input.CategoryID); err != nil {
			return nil, rssError(err)
		}
	case "rss/library/annotations":
		return p.service.ListAnnotations(input.FeedID, input.ArticleID), nil
	case "rss/library/saveAnnotation":
		var a rss.Annotation
		if err := json.Unmarshal(params, &a); err != nil {
			return nil, invalidParams("invalid annotation")
		}
		a, err := p.service.SaveAnnotation(a)
		if err != nil {
			return nil, rssError(err)
		}
		return a, nil
	case "rss/library/deleteAnnotation":
		if err := p.service.DeleteAnnotation(input.ID); err != nil {
			return nil, rssError(err)
		}
	default:
		return nil, dbxpluginsdk.MethodNotFound(method)
	}
	return map[string]any{"success": true}, nil
}
