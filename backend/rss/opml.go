package rss

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

type opmlDocument struct {
	XMLName xml.Name `xml:"opml"`
	Body    opmlBody `xml:"body"`
}
type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}
type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	URL      string        `xml:"url,attr"`
	Outlines []opmlOutline `xml:"outline"`
}
type OPMLFeed struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Group string `json:"group"`
}
type OPMLImportResult struct {
	Added   []Subscription      `json:"added"`
	Failed  []map[string]string `json:"failed"`
	Skipped []string            `json:"skipped"`
}

func ParseOPML(data []byte) ([]OPMLFeed, error) {
	if len(data) == 0 || len(data) > 2<<20 {
		return nil, errors.New("OPML 文件为空或超过 2 MiB")
	}
	var doc opmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("解析 OPML 失败：%w", err)
	}
	out := []OPMLFeed{}
	var walk func([]opmlOutline, string)
	walk = func(nodes []opmlOutline, group string) {
		for _, node := range nodes {
			url := strings.TrimSpace(node.XMLURL)
			if url == "" {
				url = strings.TrimSpace(node.URL)
			}
			next := group
			if url == "" {
				if strings.TrimSpace(node.Text) != "" {
					next = firstNonEmpty(node.Title, node.Text)
				}
				walk(node.Outlines, next)
				continue
			}
			out = append(out, OPMLFeed{URL: url, Title: firstNonEmpty(node.Title, node.Text), Group: next})
		}
	}
	walk(doc.Body.Outlines, "")
	if len(out) == 0 {
		return nil, errors.New("OPML 中没有找到订阅地址")
	}
	return out, nil
}
func (s *Service) ImportOPML(ctx context.Context, data []byte) (OPMLImportResult, error) {
	return s.ImportOPMLWithProgress(ctx, data, nil)
}

func (s *Service) ImportOPMLWithProgress(ctx context.Context, data []byte, progress func(done, total int, url string)) (OPMLImportResult, error) {
	feeds, err := ParseOPML(data)
	if err != nil {
		return OPMLImportResult{}, err
	}
	result := OPMLImportResult{}
	seen := map[string]bool{}
	for _, item := range feeds {
		if progress != nil {
			progress(len(result.Added)+len(result.Failed)+len(result.Skipped), len(feeds), item.URL)
		}
		if seen[item.URL] {
			continue
		}
		seen[item.URL] = true
		s.store.mu.Lock()
		duplicate := false
		for _, f := range s.store.data.Feeds {
			if f.URL == item.URL {
				duplicate = true
				break
			}
		}
		s.store.mu.Unlock()
		if duplicate {
			result.Skipped = append(result.Skipped, item.URL)
			continue
		}
		feed, err := s.AddFeed(ctx, item.URL)
		if err != nil {
			result.Failed = append(result.Failed, map[string]string{"url": item.URL, "error": err.Error()})
			continue
		}
		result.Added = append(result.Added, feed)
		if item.Group != "" {
			var groupID string
			for _, group := range s.ListGroups() {
				if group.Name == item.Group {
					groupID = group.ID
					break
				}
			}
			if groupID == "" {
				group, _ := s.SaveGroup("", item.Group)
				groupID = group.ID
			}
			_ = s.MoveFeed(feed.ID, groupID)
		}
	}
	if progress != nil {
		progress(len(feeds), len(feeds), "")
	}
	return result, nil
}
func (s *Service) ExportOPML(feedIDs []string) (string, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	wanted := map[string]bool{}
	for _, id := range feedIDs {
		wanted[id] = true
	}
	all := len(wanted) == 0
	groups := map[string]string{}
	for _, g := range s.store.data.Groups {
		groups[g.ID] = g.Name
	}
	type group struct {
		name  string
		feeds []Subscription
	}
	ordered := []group{}
	index := map[string]int{}
	for _, f := range s.store.data.Feeds {
		if !all && !wanted[f.ID] {
			continue
		}
		name := groups[f.GroupID]
		if name == "" {
			name = "未分组"
		}
		i, ok := index[name]
		if !ok {
			i = len(ordered)
			index[name] = i
			ordered = append(ordered, group{name: name})
		}
		ordered[i].feeds = append(ordered[i].feeds, f)
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><opml version="2.0"><head><title>RSS 阅读器订阅</title></head><body>`)
	for _, g := range ordered {
		b.WriteString(`<outline text="` + xmlEscape(g.name) + `" title="` + xmlEscape(g.name) + `">`)
		for _, f := range g.feeds {
			title := firstNonEmpty(f.Title, f.URL)
			b.WriteString(`<outline type="rss" text="` + xmlEscape(title) + `" title="` + xmlEscape(title) + `" xmlUrl="` + xmlEscape(f.URL) + `" htmlUrl="` + xmlEscape(f.Link) + `"/>`)
		}
		b.WriteString(`</outline>`)
	}
	b.WriteString(`</body></opml>`)
	return b.String(), nil
}
func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
}
