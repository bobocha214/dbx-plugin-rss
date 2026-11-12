package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/example/dbx-plugin-rss/rss"
	dbxpluginsdk "github.com/t8y2/dbx/plugins/sdk/go/dbx-plugin-sdk"
)

const (
	pluginID      = "io.github.yuzhi.rss"
	pluginVersion = "0.1.0"
)

type opmlImportJob struct {
	mu      sync.RWMutex
	Done    bool                  `json:"done"`
	Total   int                   `json:"total"`
	Current int                   `json:"current"`
	URL     string                `json:"url"`
	Result  *rss.OPMLImportResult `json:"result,omitempty"`
	Error   string                `json:"error,omitempty"`
}

type Plugin struct {
	service    *rss.Service
	jobsMu     sync.Mutex
	importJobs map[string]*opmlImportJob
}

func main() {
	dataDir, err := dbxpluginsdk.EnsureDataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ensure plugin data dir:", err)
		os.Exit(1)
	}
	store, err := rss.OpenStore(filepath.Join(dataDir, "rss", "state.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "open RSS store:", err)
		os.Exit(1)
	}
	plugin := &Plugin{service: rss.NewService(store, rss.HTTPFetcher{}), importJobs: make(map[string]*opmlImportJob)}
	metadata := dbxpluginsdk.Metadata{ID: pluginID, Version: pluginVersion, Capabilities: []string{"rss"}}
	if err := dbxpluginsdk.NewServer(metadata, plugin).Serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p *Plugin) Handle(_ dbxpluginsdk.RequestContext, method string, params json.RawMessage, _ *dbxpluginsdk.Emitter) (any, *dbxpluginsdk.PluginError) {
	if strings.HasPrefix(method, "rss/library/") {
		return p.libraryRPC(method, params)
	}
	if method == "rss/importOPMLStart" || method == "rss/importOPMLStatus" || method == "rss/importOPML" || method == "rss/exportOPML" || method == "rss/exportOPMLFile" || method == "rss/previewOPML" {
		return p.opmlRPC(method, params)
	}
	switch method {
	case "rss/listGroups":
		return p.service.ListGroups(), nil
	case "rss/getSettings":
		return p.service.GetSettings(), nil
	case "rss/listBrowsers":
		return rss.ListBrowsers(), nil
	case "rss/saveSettings":
		var input rss.Settings
		if json.Unmarshal(params, &input) != nil {
			return nil, invalidParams("invalid settings")
		}
		if err := p.service.SaveSettings(input); err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"success": true}, nil
	case "rss/saveGroup", "rss/deleteGroup", "rss/moveFeed", "rss/markAllRead":
		var input struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			FeedID  string `json:"feedId"`
			GroupID string `json:"groupId"`
		}
		if json.Unmarshal(params, &input) != nil {
			return nil, invalidParams("invalid group or scope")
		}
		switch method {
		case "rss/saveGroup":
			g, err := p.service.SaveGroup(input.ID, input.Name)
			if err != nil {
				return nil, rssError(err)
			}
			return g, nil
		case "rss/deleteGroup":
			if err := p.service.DeleteGroup(input.ID); err != nil {
				return nil, rssError(err)
			}
		case "rss/moveFeed":
			if err := p.service.MoveFeed(input.FeedID, input.GroupID); err != nil {
				return nil, rssError(err)
			}
		case "rss/markAllRead":
			n, err := p.service.MarkAllRead(input.FeedID, input.GroupID)
			if err != nil {
				return nil, rssError(err)
			}
			return map[string]any{"count": n}, nil
		}
		return map[string]any{"success": true}, nil
	case "rss/openURL", "rss/fetchImage", "rss/fetchArticle":
		var input struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(params, &input) != nil || input.URL == "" {
			return nil, invalidParams("url is required")
		}
		if method == "rss/openURL" {
			if err := rss.OpenBrowser(input.URL, p.service.GetSettings().Browser); err != nil {
				return nil, rssError(err)
			}
			return map[string]any{"success": true}, nil
		}
		if method == "rss/fetchArticle" {
			content, err := rss.FetchArticleContent(context.Background(), input.URL)
			if err != nil {
				return nil, rssError(err)
			}
			return map[string]any{"content": content}, nil
		}
		if !p.service.GetSettings().ShowImages {
			return nil, invalidParams("图片显示未启用")
		}
		image, err := rss.FetchImage(context.Background(), input.URL)
		if err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"dataURL": image}, nil
	case "rss/listFeeds":
		feeds, err := p.service.ListFeeds()
		if err != nil {
			return nil, rssError(err)
		}
		return feeds, nil
	case "rss/addFeed":
		var input struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(params, &input); err != nil || strings.TrimSpace(input.URL) == "" {
			return nil, invalidParams("url is required")
		}
		feed, err := p.service.AddFeed(context.Background(), input.URL)
		return feedOrError(feed, err)
	case "rss/refreshFeed":
		var input struct {
			FeedID string `json:"feedId"`
		}
		if err := json.Unmarshal(params, &input); err != nil || input.FeedID == "" {
			return nil, invalidParams("feedId is required")
		}
		feed, err := p.service.Refresh(context.Background(), input.FeedID)
		return feedOrError(feed, err)
	case "rss/deleteFeed":
		var input struct {
			FeedID string `json:"feedId"`
		}
		if err := json.Unmarshal(params, &input); err != nil || input.FeedID == "" {
			return nil, invalidParams("feedId is required")
		}
		if err := p.service.DeleteFeed(input.FeedID); err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"success": true}, nil
	case "rss/deleteFeeds":
		var input struct {
			FeedIDs []string `json:"feedIds"`
		}
		if err := json.Unmarshal(params, &input); err != nil || len(input.FeedIDs) == 0 {
			return nil, invalidParams("feedIds is required")
		}
		if err := p.service.DeleteFeeds(input.FeedIDs); err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"success": true, "count": len(input.FeedIDs)}, nil
	case "rss/saveArticleContent":
		var input struct {
			FeedID    string `json:"feedId"`
			ArticleID string `json:"articleId"`
			Content   string `json:"content"`
		}
		if err := json.Unmarshal(params, &input); err != nil || input.FeedID == "" || input.ArticleID == "" {
			return nil, invalidParams("feedId and articleId are required")
		}
		if err := p.service.SaveArticleContent(input.FeedID, input.ArticleID, input.Content); err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"success": true}, nil
	case "rss/listArticles":
		var input struct {
			FeedID  string `json:"feedId"`
			Unread  bool   `json:"unread"`
			GroupID string `json:"groupId"`
			Search  string `json:"search"`
			Limit   int    `json:"limit"`
			Offset  int    `json:"offset"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, invalidParams("invalid article query")
		}
		items, total, err := p.service.ListArticles(rss.ArticleQuery{FeedID: input.FeedID, GroupID: input.GroupID, Unread: input.Unread, Search: input.Search, Limit: input.Limit, Offset: input.Offset})
		if err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"items": items, "total": total}, nil
	case "rss/setRead":
		var input struct {
			FeedID    string `json:"feedId"`
			ArticleID string `json:"articleId"`
			Read      bool   `json:"read"`
		}
		if err := json.Unmarshal(params, &input); err != nil || input.FeedID == "" || input.ArticleID == "" {
			return nil, invalidParams("feedId and articleId are required")
		}
		if err := p.service.SetRead(input.FeedID, input.ArticleID, input.Read); err != nil {
			return nil, rssError(err)
		}
		return map[string]any{"success": true}, nil
	case "connection/test", "connection/connect":
		return map[string]any{"success": true, "message": "RSS 本地存储可用"}, nil
	case "connection/disconnect":
		return map[string]any{"success": true}, nil
	case "industrial/ping":
		return map[string]any{"ok": true, "plugin": pluginID, "version": pluginVersion}, nil
	default:
		return nil, dbxpluginsdk.MethodNotFound(method)
	}
}

func feedOrError(value rss.Subscription, err error) (any, *dbxpluginsdk.PluginError) {
	if err != nil {
		return nil, rssError(err)
	}
	return value, nil
}

func invalidParams(message string) *dbxpluginsdk.PluginError {
	return dbxpluginsdk.NewError(-32602, message)
}
func rssError(err error) *dbxpluginsdk.PluginError { return dbxpluginsdk.NewError(-32000, err.Error()) }

var _ dbxpluginsdk.Handler = (*Plugin)(nil)
