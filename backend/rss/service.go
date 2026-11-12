package rss

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Subscription struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Link        string    `json:"link,omitempty"`
	AddedAt     time.Time `json:"addedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	UnreadCount int       `json:"unreadCount"`
	GroupID     string    `json:"groupId,omitempty"`
}

type StoredArticle struct {
	Article
	FeedID  string    `json:"feedId"`
	IsRead  bool      `json:"isRead"`
	SavedAt time.Time `json:"savedAt"`
}

type State struct {
	Feeds       []Subscription  `json:"feeds"`
	Articles    []StoredArticle `json:"articles"`
	Groups      []Group         `json:"groups,omitempty"`
	Settings    *Settings       `json:"settings,omitempty"`
	Categories  []Group         `json:"collectionCategories,omitempty"`
	Library     []SavedArticle  `json:"savedArticles,omitempty"`
	Annotations []Annotation    `json:"annotations,omitempty"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data State
}

func OpenStore(path string) (*Store, error) {
	store := &Store{path: path, data: State{Feeds: []Subscription{}, Articles: []StoredArticle{}}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read RSS store: %w", err)
	}
	if err := json.Unmarshal(data, &store.data); err != nil {
		return nil, fmt.Errorf("decode RSS store: %w", err)
	}
	return store, nil
}

func (s *Store) snapshot() State {
	return State{Feeds: append([]Subscription(nil), s.data.Feeds...), Articles: append([]StoredArticle(nil), s.data.Articles...), Groups: append([]Group(nil), s.data.Groups...), Settings: s.data.Settings, Categories: append([]Group(nil), s.data.Categories...), Library: append([]SavedArticle(nil), s.data.Library...), Annotations: append([]Annotation(nil), s.data.Annotations...)}
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

type FeedFetcher interface {
	Fetch(context.Context, string, string, string) (FetchedFeed, error)
}

type FetchedFeed struct{ Feed Feed }

type HTTPFetcher struct{ Client *http.Client }

func (f HTTPFetcher) Fetch(ctx context.Context, sourceURL, etag, lastModified string) (FetchedFeed, error) {
	if err := ValidateFeedURL(sourceURL); err != nil {
		return FetchedFeed{}, err
	}
	client := f.Client
	if client == nil {
		client = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return ValidateFeedURL(req.URL.String())
		}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return FetchedFeed{}, err
	}
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml;q=0.9")
	req.Header.Set("User-Agent", "DBX RSS Plugin/0.1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := client.Do(req)
	if err != nil {
		return FetchedFeed{}, fmt.Errorf("fetch RSS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FetchedFeed{}, fmt.Errorf("RSS server returned HTTP %d", resp.StatusCode)
	}
	feed, err := Parse(io.LimitReader(resp.Body, maxFeedBytes+1), sourceURL)
	if err != nil {
		return FetchedFeed{}, err
	}
	return FetchedFeed{Feed: feed}, nil
}

type Service struct {
	store   *Store
	fetcher FeedFetcher
}

func NewService(store *Store, fetcher FeedFetcher) *Service {
	return &Service{store: store, fetcher: fetcher}
}

func (s *Service) AddFeed(ctx context.Context, sourceURL string) (Subscription, error) {
	if err := ValidateFeedURL(sourceURL); err != nil {
		return Subscription{}, err
	}
	s.store.mu.Lock()
	for _, existing := range s.store.data.Feeds {
		if existing.URL == sourceURL {
			s.store.mu.Unlock()
			return Subscription{}, errors.New("该订阅已存在")
		}
	}
	s.store.mu.Unlock()
	fetched, err := s.fetch(ctx, sourceURL)
	if err != nil {
		return Subscription{}, err
	}
	now := time.Now().UTC()
	feed := Subscription{ID: stableID(sourceURL, "", time.Time{}), URL: sourceURL, Title: firstNonEmpty(fetched.Feed.Title, sourceURL), Description: fetched.Feed.Description, Link: fetched.Feed.Link, AddedAt: now, UpdatedAt: now}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	s.store.data.Feeds = append(s.store.data.Feeds, feed)
	s.mergeArticlesLocked(feed.ID, fetched.Feed.Items, now)
	s.trimArticlesLocked(feed.ID)
	if err := s.store.saveLocked(); err != nil {
		return Subscription{}, fmt.Errorf("save RSS subscription: %w", err)
	}
	return feed, nil
}

func (s *Service) Refresh(ctx context.Context, feedID string) (Subscription, error) {
	s.store.mu.Lock()
	var feed *Subscription
	for i := range s.store.data.Feeds {
		if s.store.data.Feeds[i].ID == feedID {
			copy := s.store.data.Feeds[i]
			feed = &copy
			break
		}
	}
	s.store.mu.Unlock()
	if feed == nil {
		return Subscription{}, errors.New("订阅不存在")
	}
	fetched, err := s.fetch(ctx, feed.URL)
	if err != nil {
		return Subscription{}, err
	}
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	for i := range s.store.data.Feeds {
		if s.store.data.Feeds[i].ID == feedID {
			s.store.data.Feeds[i].Title = firstNonEmpty(fetched.Feed.Title, feed.URL)
			s.store.data.Feeds[i].Description = fetched.Feed.Description
			s.store.data.Feeds[i].Link = fetched.Feed.Link
			s.store.data.Feeds[i].UpdatedAt = time.Now().UTC()
			feed = &s.store.data.Feeds[i]
			break
		}
	}
	s.mergeArticlesLocked(feedID, fetched.Feed.Items, time.Now().UTC())
	s.trimArticlesLocked(feedID)
	s.recountLocked(feedID)
	if err := s.store.saveLocked(); err != nil {
		return Subscription{}, err
	}
	return *feed, nil
}

func (s *Service) DeleteFeed(feedID string) error {
	return s.DeleteFeeds([]string{feedID})
}

func (s *Service) DeleteFeeds(feedIDs []string) error {
	wanted := make(map[string]bool, len(feedIDs))
	for _, id := range feedIDs {
		if id != "" {
			wanted[id] = true
		}
	}
	if len(wanted) == 0 {
		return errors.New("请选择要删除的订阅")
	}
	return s.change(func() error {
		found := make(map[string]bool, len(wanted))
		for _, feed := range s.store.data.Feeds {
			if wanted[feed.ID] {
				found[feed.ID] = true
			}
		}
		if len(found) != len(wanted) {
			return errors.New("所选订阅中包含已不存在的项目")
		}
		feeds := make([]Subscription, 0, len(s.store.data.Feeds)-len(wanted))
		for _, feed := range s.store.data.Feeds {
			if !wanted[feed.ID] {
				feeds = append(feeds, feed)
			}
		}
		articles := make([]StoredArticle, 0, len(s.store.data.Articles))
		for _, article := range s.store.data.Articles {
			if !wanted[article.FeedID] {
				articles = append(articles, article)
			}
		}
		s.store.data.Feeds, s.store.data.Articles = feeds, articles
		return nil
	})
}

func (s *Service) SaveArticleContent(feedID, articleID, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("文章正文为空")
	}
	return s.change(func() error {
		found := false
		for i := range s.store.data.Articles {
			if s.store.data.Articles[i].FeedID == feedID && s.store.data.Articles[i].ID == articleID {
				s.store.data.Articles[i].Content = content
				found = true
			}
		}
		for i := range s.store.data.Library {
			if s.store.data.Library[i].FeedID == feedID && s.store.data.Library[i].ID == articleID {
				s.store.data.Library[i].Content = content
			}
		}
		if !found {
			return errors.New("文章不存在")
		}
		return nil
	})
}

func (s *Service) SetRead(feedID, articleID string, read bool) error {
	return s.change(func() error {
		found := false
		for i := range s.store.data.Library {
			a := &s.store.data.Library[i]
			if a.FeedID == feedID && a.ID == articleID {
				a.IsRead = read
				found = true
			}
		}
		for i := range s.store.data.Articles {
			if s.store.data.Articles[i].FeedID == feedID && s.store.data.Articles[i].ID == articleID {
				s.store.data.Articles[i].IsRead = read
				s.recountLocked(feedID)
				return nil
			}
		}
		if found {
			return nil
		}
		return errors.New("文章不存在")
	})
}

type ArticleQuery struct {
	FeedID  string
	GroupID string
	Unread  bool
	Search  string
	Limit   int
	Offset  int
}

func (s *Service) ListFeeds() ([]Subscription, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	result := make([]Subscription, len(s.store.data.Feeds))
	copy(result, s.store.data.Feeds)
	for i := range result {
		result[i].UnreadCount = 0
		for _, article := range s.store.data.Articles {
			if article.FeedID == result[i].ID && !article.IsRead {
				result[i].UnreadCount++
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}

func (s *Service) ListArticles(query ArticleQuery) ([]StoredArticle, int, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	search := strings.ToLower(strings.TrimSpace(query.Search))
	all := make([]StoredArticle, 0)
	for _, article := range s.store.data.Articles {
		if !s.inScopeLocked(article.FeedID, query.FeedID, query.GroupID) || query.Unread && article.IsRead {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(article.Title+" "+article.Summary+" "+article.Content), search) {
			continue
		}
		all = append(all, article)
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].PublishedAt.After(all[j].PublishedAt)
	})
	total := len(all)
	offset := max(query.Offset, 0)
	limit := query.Limit
	if offset >= total {
		return []StoredArticle{}, total, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return all[offset:end], total, nil
}

func (s *Service) mergeArticlesLocked(feedID string, items []Article, now time.Time) {
	existing := make(map[string]int, len(s.store.data.Articles))
	for i := range s.store.data.Articles {
		existing[feedKey(s.store.data.Articles[i].FeedID, s.store.data.Articles[i].ID)] = i
	}
	for _, item := range items {
		for i := range s.store.data.Library {
			if s.store.data.Library[i].FeedID == feedID && s.store.data.Library[i].ID == item.ID {
				s.store.data.Library[i].Article = item
			}
		}
		key := feedKey(feedID, item.ID)
		if index, ok := existing[key]; ok {
			s.store.data.Articles[index].Article = item
			continue
		}
		s.store.data.Articles = append(s.store.data.Articles, StoredArticle{Article: item, FeedID: feedID, SavedAt: now})
	}
}

func (s *Service) recountLocked(feedID string) {
	for i := range s.store.data.Feeds {
		if s.store.data.Feeds[i].ID != feedID {
			continue
		}
		count := 0
		for _, article := range s.store.data.Articles {
			if article.FeedID == feedID && !article.IsRead {
				count++
			}
		}
		s.store.data.Feeds[i].UnreadCount = count
	}
}

func feedKey(feedID, articleID string) string { return feedID + "\x00" + articleID }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
