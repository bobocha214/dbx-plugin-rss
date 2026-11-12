package rss

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Settings struct {
	DisplayMode    string  `json:"displayMode"`
	ShowImages     bool    `json:"showImages"`
	ImageWidth     int     `json:"imageWidth"`
	Density        string  `json:"density"`
	TimeoutSeconds int     `json:"timeoutSeconds"`
	MaxArticles    int     `json:"maxArticles"`
	Browser        string  `json:"browser"`
	ReaderTheme    string  `json:"readerTheme"`
	FontSize       int     `json:"fontSize"`
	LineHeight     float64 `json:"lineHeight"`
	ReaderWidth    int     `json:"readerWidth"`
}

func DefaultSettings() Settings {
	return Settings{DisplayMode: "rich", ShowImages: true, ImageWidth: 800, Density: "comfortable", TimeoutSeconds: 30, MaxArticles: 0, Browser: "default", ReaderTheme: "auto", FontSize: 17, LineHeight: 1.9, ReaderWidth: 900}
}
func normalizeReading(v Settings) Settings {
	if v.ReaderTheme == "" {
		v.ReaderTheme = "auto"
	}
	if v.FontSize == 0 {
		v.FontSize = 17
	}
	if v.LineHeight == 0 {
		v.LineHeight = 1.9
	}
	if v.ReaderWidth == 0 {
		v.ReaderWidth = 900
	}
	return v
}
func (s *Service) GetSettings() Settings {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if s.store.data.Settings == nil {
		return DefaultSettings()
	}
	return normalizeReading(*s.store.data.Settings)
}
func (s *Service) SaveSettings(v Settings) error {
	v = normalizeReading(v)
	if v.ReaderTheme != "auto" && v.ReaderTheme != "light" && v.ReaderTheme != "dark" && v.ReaderTheme != "sepia" && v.ReaderTheme != "green" {
		return errors.New("无效的阅读配色")
	}
	if v.FontSize < 14 || v.FontSize > 28 || v.LineHeight < 1.5 || v.LineHeight > 2.4 || v.ReaderWidth < 600 || v.ReaderWidth > 1200 {
		return errors.New("字号 14–28，行距 1.5–2.4，正文宽度 600–1200")
	}
	if v.DisplayMode != "rich" && v.DisplayMode != "text" && v.DisplayMode != "summary" {
		return errors.New("无效的阅读模式")
	}
	if v.Density != "comfortable" && v.Density != "compact" {
		return errors.New("无效的列表密度")
	}
	if v.TimeoutSeconds < 5 || v.TimeoutSeconds > 120 {
		return errors.New("请求超时应为 5–120 秒")
	}
	if v.ImageWidth < 200 || v.ImageWidth > 1600 {
		return errors.New("图片宽度应为 200–1600 像素")
	}
	if v.MaxArticles < 0 || v.MaxArticles > 10000 {
		return errors.New("每源保留数量应为 0–10000，0 表示不限")
	}
	if !validBrowser(v.Browser) {
		return errors.New("无效的浏览器选择")
	}
	if v.Browser != "default" {
		found := false
		for _, b := range ListBrowsers() {
			if b.ID == v.Browser {
				found = true
			}
		}
		if !found {
			return errors.New("所选浏览器未安装")
		}
	}
	return s.change(func() error { s.store.data.Settings = &v; return nil })
}

// All management writes roll back in memory if the disk write fails.
func (s *Service) change(fn func() error) error {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	before := s.store.snapshot()
	if err := fn(); err != nil {
		s.store.data = before
		return err
	}
	if err := s.store.saveLocked(); err != nil {
		s.store.data = before
		return err
	}
	return nil
}
func (s *Service) ListGroups() []Group {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return append([]Group{}, s.store.data.Groups...)
}
func (s *Service) SaveGroup(id, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return Group{}, errors.New("分组名称应为 1–60 个字符")
	}
	result := Group{ID: id, Name: name}
	err := s.change(func() error {
		for _, g := range s.store.data.Groups {
			if g.Name == name && g.ID != id {
				return errors.New("分组名称已存在")
			}
		}
		if id == "" {
			result.ID = rand.Text()
			s.store.data.Groups = append(s.store.data.Groups, result)
			return nil
		}
		for i, g := range s.store.data.Groups {
			if g.ID == id {
				s.store.data.Groups[i] = result
				return nil
			}
		}
		return errors.New("分组不存在")
	})
	return result, err
}
func (s *Service) DeleteGroup(id string) error {
	return s.change(func() error {
		found := false
		groups := []Group{}
		for _, g := range s.store.data.Groups {
			if g.ID == id {
				found = true
			} else {
				groups = append(groups, g)
			}
		}
		if !found {
			return errors.New("分组不存在")
		}
		s.store.data.Groups = groups
		for i := range s.store.data.Feeds {
			if s.store.data.Feeds[i].GroupID == id {
				s.store.data.Feeds[i].GroupID = ""
			}
		}
		return nil
	})
}
func (s *Service) MoveFeed(feedID, groupID string) error {
	return s.change(func() error {
		if groupID != "" {
			found := false
			for _, g := range s.store.data.Groups {
				if g.ID == groupID {
					found = true
				}
			}
			if !found {
				return errors.New("分组不存在")
			}
		}
		for i := range s.store.data.Feeds {
			if s.store.data.Feeds[i].ID == feedID {
				s.store.data.Feeds[i].GroupID = groupID
				return nil
			}
		}
		return errors.New("订阅不存在")
	})
}

// "ungrouped" is a UI scope, never a persisted group ID.
func (s *Service) inScopeLocked(feedID, scopeFeed, groupID string) bool {
	if scopeFeed != "" && feedID != scopeFeed {
		return false
	}
	if groupID == "" {
		return true
	}
	for _, f := range s.store.data.Feeds {
		if f.ID == feedID {
			return f.GroupID == groupID || groupID == "ungrouped" && f.GroupID == ""
		}
	}
	return false
}
func (s *Service) MarkAllRead(feedID, groupID string) (int, error) {
	n := 0
	err := s.change(func() error {
		if feedID != "" {
			found := false
			for _, f := range s.store.data.Feeds {
				if f.ID == feedID {
					found = true
				}
			}
			if !found {
				return errors.New("订阅不存在")
			}
		}
		if groupID != "" && groupID != "ungrouped" {
			found := false
			for _, g := range s.store.data.Groups {
				if g.ID == groupID {
					found = true
				}
			}
			if !found {
				return errors.New("分组不存在")
			}
		}
		for i, a := range s.store.data.Articles {
			if !a.IsRead && s.inScopeLocked(a.FeedID, feedID, groupID) {
				s.store.data.Articles[i].IsRead = true
				n++
			}
		}
		return nil
	})
	return n, err
}
func (s *Service) fetch(ctx context.Context, url string) (FetchedFeed, error) {
	timeout := s.GetSettings().TimeoutSeconds
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	result, err := s.fetcher.Fetch(ctx, url, "", "")
	if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
		return result, fmt.Errorf("获取订阅超时（%d 秒）：请检查网络、代理或订阅源是否可访问，也可在设置中增加超时时间", timeout)
	}
	return result, err
}
func (s *Service) trimArticlesLocked(feedID string) {
	cap := 0
	if s.store.data.Settings != nil {
		cap = s.store.data.Settings.MaxArticles
	}
	if cap == 0 {
		return
	}
	articles := []StoredArticle{}
	other := []StoredArticle{}
	for _, a := range s.store.data.Articles {
		if a.FeedID == feedID {
			articles = append(articles, a)
		} else {
			other = append(other, a)
		}
	}
	sort.SliceStable(articles, func(i, j int) bool {
		if articles[i].PublishedAt.Equal(articles[j].PublishedAt) {
			return articles[i].SavedAt.After(articles[j].SavedAt)
		}
		return articles[i].PublishedAt.After(articles[j].PublishedAt)
	})
	if len(articles) > cap {
		articles = articles[:cap]
	}
	s.store.data.Articles = append(other, articles...)
}
