package rss

import (
	"crypto/rand"
	"errors"
	"strings"
	"time"
	"unicode/utf16"
)

type Annotation struct {
	ID        string    `json:"id"`
	FeedID    string    `json:"feedId"`
	ArticleID string    `json:"articleId"`
	Kind      string    `json:"kind"`
	Color     string    `json:"color"`
	Quote     string    `json:"quote"`
	Prefix    string    `json:"prefix"`
	Suffix    string    `json:"suffix"`
	Start     int       `json:"start"`
	End       int       `json:"end"`
	Mode      string    `json:"mode"`
	Note      string    `json:"note"`
	UpdatedAt time.Time `json:"updatedAt"`
	Underline bool      `json:"underline"`
	// SurfaceText is transient renderer text used only to resolve overlap in the
	// current display mode. It never replaces the trusted article snapshot.
	SurfaceText string `json:"surfaceText,omitempty"`
}

func resolveAnnotation(surface string, a Annotation, mode string) (int, int, bool) {
	if a.Quote == "" || a.Mode != mode {
		return 0, 0, false
	}
	found := -1
	for offset := 0; offset <= len(surface); {
		rel := strings.Index(surface[offset:], a.Quote)
		if rel < 0 {
			break
		}
		at := offset + rel
		end := at + len(a.Quote)
		if strings.HasSuffix(surface[:at], a.Prefix) && strings.HasPrefix(surface[end:], a.Suffix) {
			if found >= 0 {
				return 0, 0, false
			}
			found = at
		}
		offset = at + 1
	}
	if found < 0 {
		return 0, 0, false
	}
	start := len(utf16.Encode([]rune(surface[:found])))
	return start, start + len(utf16.Encode([]rune(a.Quote))), true
}

func (s *Service) ListAnnotations(feedID, articleID string) []Annotation {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	out := []Annotation{}
	for _, a := range s.store.data.Annotations {
		if a.FeedID == feedID && a.ArticleID == articleID {
			out = append(out, a)
		}
	}
	return out
}
func (s *Service) SaveAnnotation(a Annotation) (Annotation, error) {
	surface := a.SurfaceText
	a.SurfaceText = ""
	if a.FeedID == "" || a.ArticleID == "" {
		return a, errors.New("文章标识不能为空")
	}
	a.Note = strings.TrimSpace(a.Note)
	if len(surface) > 6<<20 {
		return a, errors.New("正文过长")
	}
	surfaceValid := false
	if len([]rune(a.Note)) > 20000 || len(a.Prefix) > 512 || len(a.Suffix) > 512 || len(a.Quote) > 40000 {
		return a, errors.New("笔记或选中文字过长")
	}
	switch a.Kind {
	case "note":
		if a.Note == "" || a.Quote != "" {
			return a, errors.New("请输入文章笔记")
		}
	case "highlight", "color", "underline":
		if a.Quote == "" || a.Start < 0 || a.End <= a.Start || a.End > 10000000 || a.End-a.Start != len(utf16.Encode([]rune(a.Quote))) {
			return a, errors.New("无效的文字选区")
		}
		if a.Mode != "rich" && a.Mode != "text" && a.Mode != "summary" {
			return a, errors.New("无效的正文模式")
		}
		units := utf16.Encode([]rune(surface))
		surfaceValid = a.End <= len(units) && string(utf16.Decode(units[a.Start:a.End])) == a.Quote
		if a.Kind == "highlight" && a.Color != "yellow" && a.Color != "green" && a.Color != "blue" && a.Color != "pink" {
			return a, errors.New("无效的高亮颜色")
		}
		if a.Kind == "color" && a.Color != "red" && a.Color != "blue" {
			return a, errors.New("无效的文字颜色")
		}
		if a.Kind == "underline" && a.Color != "" {
			return a, errors.New("下划线无需颜色")
		}
	default:
		return a, errors.New("无效的批注类型")
	}
	err := s.change(func() error {
		article, err := s.libraryArticleLocked(a.FeedID, a.ArticleID)
		if err != nil {
			return err
		}
		index := -1
		for i, v := range s.store.data.Annotations {
			if v.ID == a.ID {
				if v.FeedID != a.FeedID || v.ArticleID != a.ArticleID {
					return errors.New("批注不属于该文章")
				}
				index = i
				break
			}
		}
		if a.ID != "" && index < 0 {
			return errors.New("批注不存在")
		}
		if a.Kind != "note" {
			if !surfaceValid {
				if index < 0 {
					return errors.New("正文选区已变化，请重新选择")
				}
				old := s.store.data.Annotations[index]
				if old.Quote != a.Quote || old.Start != a.Start || old.End != a.End || old.Mode != a.Mode || old.Prefix != a.Prefix || old.Suffix != a.Suffix {
					return errors.New("正文选区已变化，请重新选择")
				}
			}
			for _, v := range s.store.data.Annotations {
				if surfaceValid && v.ID != a.ID && v.Kind != "note" && v.FeedID == a.FeedID && v.ArticleID == a.ArticleID {
					start, end, ok := resolveAnnotation(surface, v, a.Mode)
					if ok && start < a.End && a.Start < end {
						return errors.New("选区与已有标记重叠，请编辑已有标记")
					}
				}
			}
		}
		a.UpdatedAt = time.Now().UTC()
		if a.ID == "" {
			a.ID = rand.Text()
		}
		if index < 0 {
			s.store.data.Annotations = append(s.store.data.Annotations, a)
		} else {
			s.store.data.Annotations[index] = a
		}
		s.snapshotArticleLocked(article)
		return nil
	})
	return a, err
}
func (s *Service) DeleteAnnotation(id string) error {
	return s.change(func() error {
		out := []Annotation{}
		found := false
		for _, a := range s.store.data.Annotations {
			if a.ID == id {
				found = true
			} else {
				out = append(out, a)
			}
		}
		if !found {
			return errors.New("批注不存在")
		}
		s.store.data.Annotations = out
		return nil
	})
}
