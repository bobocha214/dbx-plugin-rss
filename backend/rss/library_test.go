package rss

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepeatedNotesAndRenamedCategoriesHaveDistinctIDs(t *testing.T) {
	s, f := libraryService(t)
	a, err := s.SaveAnnotation(Annotation{FeedID: f.ID, ArticleID: "one", Kind: "note", Note: "same"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.SaveAnnotation(Annotation{FeedID: f.ID, ArticleID: "one", Kind: "note", Note: "same"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Fatal("separate notes share an ID")
	}
	g, _ := s.SaveCategory("", "学习")
	if _, err = s.SaveCategory(g.ID, "阅读"); err != nil {
		t.Fatal(err)
	}
	h, err := s.SaveCategory("", "学习")
	if err != nil {
		t.Fatal(err)
	}
	if h.ID == g.ID {
		t.Fatal("renamed category reused ID")
	}
}

func TestNotesLibrarySearchAndPreview(t *testing.T) {
	s, f := libraryService(t)
	_, err := s.SaveAnnotation(Annotation{FeedID: f.ID, ArticleID: "one", Kind: "note", Note: "needle in a note"})
	if err != nil {
		t.Fatal(err)
	}
	items, total := s.ListLibrary(LibraryQuery{Notes: true, Search: "needle", Limit: 1})
	if total != 1 || items[0].NotePreview != "needle in a note" {
		t.Fatalf("missing note preview: %+v", items)
	}
	_, total = s.ListLibrary(LibraryQuery{Notes: true, Search: "absent"})
	if total != 0 {
		t.Fatal("search ignored")
	}
	items, total = s.ListLibrary(LibraryQuery{Notes: true, Offset: 1, Limit: 1})
	if total != 1 || len(items) != 0 {
		t.Fatal("bad pagination")
	}
}

func libraryService(t *testing.T) (*Service, Subscription) {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(store, fakeFetcher{feed: Feed{Title: "Source", Items: []Article{{ID: "one", Title: "文章", Content: "Hello world"}}}})
	f, err := s.AddFeed(context.Background(), "https://example.com/rss")
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}
func TestSavedArticleAndNoteSurviveDeletion(t *testing.T) {
	s, f := libraryService(t)
	g, err := s.SaveCategory("", "学习")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetFavorite(f.ID, "one", true, g.ID); err != nil {
		t.Fatal(err)
	}
	note, err := s.SaveAnnotation(Annotation{FeedID: f.ID, ArticleID: "one", Kind: "note", Note: "我的笔记"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteFeed(f.ID); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(s.store.path)
	if err != nil {
		t.Fatal(err)
	}
	s = NewService(store, fakeFetcher{})
	items, total := s.ListLibrary(LibraryQuery{Favorites: true})
	if total != 1 || items[0].Content != "Hello world" || items[0].FeedTitle != "Source" {
		t.Fatalf("lost snapshot: %+v", items)
	}
	if err = s.DeleteCategory(g.ID); err != nil {
		t.Fatal(err)
	}
	items, _ = s.ListLibrary(LibraryQuery{Favorites: true})
	if items[0].CategoryID != "" {
		t.Fatal("category was not cleared")
	}
	if err = s.SetFavorite(f.ID, "one", false, ""); err != nil {
		t.Fatal(err)
	}
	items, total = s.ListLibrary(LibraryQuery{Notes: true})
	if total != 1 || len(s.ListAnnotations(f.ID, "one")) != 1 {
		t.Fatal("unfavorite removed notes")
	}
	if err = s.DeleteAnnotation(note.ID); err != nil {
		t.Fatal(err)
	}
	_, total = s.ListLibrary(LibraryQuery{Notes: true})
	if total != 0 {
		t.Fatal("deleted note remains")
	}
}
func TestAnnotationValidationAndRollback(t *testing.T) {
	s, f := libraryService(t)
	mark := Annotation{FeedID: f.ID, ArticleID: "one", Kind: "highlight", Color: "yellow", Quote: "Hello", Start: 0, End: 5, Mode: "rich", SurfaceText: "Hello world"}
	saved, err := s.SaveAnnotation(mark)
	if err != nil {
		t.Fatal(err)
	}
	bad := mark
	bad.Color = "url(javascript:bad)"
	if _, err = s.SaveAnnotation(bad); err == nil {
		t.Fatal("accepted arbitrary style")
	}
	bad = mark
	bad.End = 0
	if _, err = s.SaveAnnotation(bad); err == nil {
		t.Fatal("accepted empty range")
	}
	bad = mark
	bad.FeedID = "missing"
	if _, err = s.SaveAnnotation(bad); err == nil {
		t.Fatal("accepted unknown article")
	}
	s.store.path = filepath.Join(t.TempDir(), "block", "state.json")
	if err = os.WriteFile(filepath.Dir(s.store.path), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	saved.Note = "changed"
	if _, err = s.SaveAnnotation(saved); err == nil {
		t.Fatal("expected disk failure")
	}
	if s.ListAnnotations(f.ID, "one")[0].Note != "" {
		t.Fatal("failed save changed memory")
	}
}

func TestOverlapUsesCurrentSurfaceAndPreservesUnderline(t *testing.T) {
	s, f := libraryService(t)
	a, err := s.SaveAnnotation(Annotation{FeedID: f.ID, ArticleID: "one", Kind: "color", Color: "red", Underline: true, Quote: "Hello", Suffix: " world", Start: 0, End: 5, Mode: "rich", SurfaceText: "Hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.Underline || a.Color != "red" || a.SurfaceText != "" {
		t.Fatal("style lost or validation text persisted")
	}
	next := Annotation{FeedID: f.ID, ArticleID: "one", Kind: "highlight", Color: "yellow", Quote: "new", Suffix: " new Hello world", Start: 0, End: 3, Mode: "rich", SurfaceText: "new new Hello world"}
	if _, err = s.SaveAnnotation(next); err != nil {
		t.Fatalf("historical offsets blocked new selection: %v", err)
	}
	next.Quote = "Hello "
	next.Start = 8
	next.End = 14
	next.Suffix = "world"
	if _, err = s.SaveAnnotation(next); err == nil {
		t.Fatal("accepted overlap after text shifted")
	}
}

func TestOrphanAnnotationCanBeEditedWithoutMovingAnchor(t *testing.T) {
	s, f := libraryService(t)
	a, err := s.SaveAnnotation(Annotation{FeedID: f.ID, ArticleID: "one", Kind: "highlight", Color: "yellow", Quote: "Hello", Start: 0, End: 5, Mode: "rich", SurfaceText: "Hello world"})
	if err != nil {
		t.Fatal(err)
	}
	a.Note = "原句已删除，保留想法"
	a.SurfaceText = "changed document"
	updated, err := s.SaveAnnotation(a)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Start != 0 || updated.Quote != "Hello" || updated.Note != a.Note {
		t.Fatal("orphan anchor moved")
	}
	a.Start = 1
	a.End = 6
	if _, err = s.SaveAnnotation(a); err == nil {
		t.Fatal("accepted invalid relocation")
	}
}
func TestLibrarySurvivesRetentionAndRefresh(t *testing.T) {
	s, f := libraryService(t)
	if err := s.SetFavorite(f.ID, "one", true, ""); err != nil {
		t.Fatal(err)
	}
	settings := s.GetSettings()
	settings.MaxArticles = 1
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	s.fetcher = fakeFetcher{feed: Feed{Items: []Article{{ID: "one", Title: "更新", Content: "new"}, {ID: "two"}}}}
	if _, err := s.Refresh(context.Background(), f.ID); err != nil {
		t.Fatal(err)
	}
	items, _ := s.ListLibrary(LibraryQuery{Favorites: true})
	if items[0].Content != "new" {
		t.Fatal("snapshot not refreshed")
	}
}
func TestReadingSettingsMigrationAndBounds(t *testing.T) {
	s, _ := libraryService(t)
	old := s.GetSettings()
	old.ReaderTheme = ""
	old.FontSize = 0
	old.LineHeight = 0
	old.ReaderWidth = 0
	if err := s.SaveSettings(old); err != nil {
		t.Fatal(err)
	}
	v := s.GetSettings()
	if v.ReaderTheme != "auto" || v.FontSize != 17 || v.ReaderWidth != 900 {
		t.Fatalf("bad migration %+v", v)
	}
	v.ReaderTheme = "sepia"
	v.FontSize = 28
	v.LineHeight = 2.4
	v.ReaderWidth = 1200
	if err := s.SaveSettings(v); err != nil {
		t.Fatal(err)
	}
	v.ReaderTheme = "arbitrary"
	if s.SaveSettings(v) == nil {
		t.Fatal("accepted theme")
	}
}
