package rss

import (
	"crypto/rand"
	"errors"
	"sort"
	"strings"
	"time"
)

type SavedArticle struct {
	StoredArticle
	FeedTitle   string    `json:"feedTitle"`
	FeedURL     string    `json:"feedUrl"`
	IsFavorite  bool      `json:"isFavorite"`
	CategoryID  string    `json:"categoryId"`
	CollectedAt time.Time `json:"collectedAt"`
	NotePreview string    `json:"notePreview,omitempty"`
	LastNoteAt  time.Time `json:"lastNoteAt,omitempty"`
}
type LibraryQuery struct {
	Favorites, Notes   bool
	CategoryID, Search string
	Limit, Offset      int
}
type LibraryIndex struct {
	FeedID     string `json:"feedId"`
	ArticleID  string `json:"articleId"`
	IsFavorite bool   `json:"isFavorite"`
	CategoryID string `json:"categoryId"`
	NoteCount  int    `json:"noteCount"`
}

func (s *Service) ListCategories() []Group {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return append([]Group{}, s.store.data.Categories...)
}
func (s *Service) SaveCategory(id, name string) (Group, error) {
	name = strings.TrimSpace(name)
	g := Group{ID: id, Name: name}
	if name == "" || len([]rune(name)) > 60 {
		return g, errors.New("分类名称应为 1–60 个字符")
	}
	err := s.change(func() error {
		for _, v := range s.store.data.Categories {
			if v.Name == name && v.ID != id {
				return errors.New("分类名称已存在")
			}
		}
		if id == "" {
			g.ID = rand.Text()
			s.store.data.Categories = append(s.store.data.Categories, g)
			return nil
		}
		for i, v := range s.store.data.Categories {
			if v.ID == id {
				s.store.data.Categories[i] = g
				return nil
			}
		}
		return errors.New("分类不存在")
	})
	return g, err
}
func (s *Service) DeleteCategory(id string) error {
	return s.change(func() error {
		found := false
		out := []Group{}
		for _, g := range s.store.data.Categories {
			if g.ID == id {
				found = true
			} else {
				out = append(out, g)
			}
		}
		if !found {
			return errors.New("分类不存在")
		}
		s.store.data.Categories = out
		for i := range s.store.data.Library {
			if s.store.data.Library[i].CategoryID == id {
				s.store.data.Library[i].CategoryID = ""
			}
		}
		return nil
	})
}
func (s *Service) libraryArticleLocked(feedID, articleID string) (SavedArticle, error) {
	var saved SavedArticle
	hasSaved := false
	for _, a := range s.store.data.Library {
		if a.FeedID == feedID && a.ID == articleID {
			saved = a
			hasSaved = true
			break
		}
	}
	for _, a := range s.store.data.Articles {
		if a.FeedID == feedID && a.ID == articleID {
			saved.StoredArticle = a
			for _, f := range s.store.data.Feeds {
				if f.ID == feedID {
					saved.FeedTitle = f.Title
					saved.FeedURL = f.URL
					break
				}
			}
			return saved, nil
		}
	}
	if hasSaved {
		return saved, nil
	}
	return saved, errors.New("文章不存在")
}
func (s *Service) snapshotArticleLocked(a SavedArticle) {
	for i, v := range s.store.data.Library {
		if a.ID == v.ID && a.FeedID == v.FeedID {
			s.store.data.Library[i] = a
			return
		}
	}
	s.store.data.Library = append(s.store.data.Library, a)
}
func (s *Service) GetArticle(feedID, articleID string) (SavedArticle, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return s.libraryArticleLocked(feedID, articleID)
}
func (s *Service) SetFavorite(feedID, articleID string, favorite bool, category string) error {
	return s.change(func() error {
		if category != "" {
			found := false
			for _, g := range s.store.data.Categories {
				if g.ID == category {
					found = true
				}
			}
			if !found {
				return errors.New("收藏分类不存在")
			}
		}
		a, err := s.libraryArticleLocked(feedID, articleID)
		if err != nil {
			return err
		}
		if favorite && !a.IsFavorite {
			a.CollectedAt = time.Now().UTC()
		}
		a.IsFavorite = favorite
		a.CategoryID = category
		s.snapshotArticleLocked(a)
		return nil
	})
}
func (s *Service) LibraryIndex() []LibraryIndex {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	out := []LibraryIndex{}
	for _, a := range s.store.data.Library {
		n := 0
		for _, v := range s.store.data.Annotations {
			if v.FeedID == a.FeedID && v.ArticleID == a.ID {
				n++
			}
		}
		if a.IsFavorite || n > 0 {
			out = append(out, LibraryIndex{a.FeedID, a.ID, a.IsFavorite, a.CategoryID, n})
		}
	}
	return out
}
func (s *Service) ListLibrary(q LibraryQuery) ([]SavedArticle, int) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	out := []SavedArticle{}
	search := strings.ToLower(strings.TrimSpace(q.Search))
	for _, stored := range s.store.data.Library {
		a, _ := s.libraryArticleLocked(stored.FeedID, stored.ID)
		if q.Favorites && !a.IsFavorite {
			continue
		}
		if q.CategoryID != "" && a.CategoryID != q.CategoryID && !(q.CategoryID == "uncategorized" && a.CategoryID == "") {
			continue
		}
		n := 0
		text := a.Title + " " + a.Summary + " " + a.Content
		for _, v := range s.store.data.Annotations {
			if v.FeedID == a.FeedID && v.ArticleID == a.ID {
				n++
				text += " " + v.Note + " " + v.Quote
				if v.UpdatedAt.After(a.LastNoteAt) {
					a.LastNoteAt = v.UpdatedAt
					a.NotePreview = strings.TrimSpace(v.Quote + " " + v.Note)
				}
			}
		}
		if q.Notes && n == 0 {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(text), search) {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if q.Notes {
			return out[i].LastNoteAt.After(out[j].LastNoteAt)
		}
		return out[i].CollectedAt.After(out[j].CollectedAt)
	})
	total := len(out)
	offset := max(q.Offset, 0)
	if offset >= total {
		return []SavedArticle{}, total
	}
	end := total
	if q.Limit > 0 && offset+q.Limit < end {
		end = offset + q.Limit
	}
	return out[offset:end], total
}
