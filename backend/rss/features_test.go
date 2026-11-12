package rss

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGroupsAndBulkReadPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, _ := OpenStore(path)
	s := NewService(store, fakeFetcher{feed: Feed{Items: []Article{{ID: "a"}, {ID: "b"}, {ID: "c"}}}})
	a, _ := s.AddFeed(context.Background(), "https://example.com/a")
	b, _ := s.AddFeed(context.Background(), "https://example.com/b")
	g, err := s.SaveGroup("", "技术")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MoveFeed(a.ID, g.ID); err != nil {
		t.Fatal(err)
	}
	_, total, _ := s.ListArticles(ArticleQuery{GroupID: g.ID, Limit: 1})
	if total != 3 {
		t.Fatalf("group total=%d", total)
	}
	n, err := s.MarkAllRead(a.ID, "")
	if err != nil || n != 3 {
		t.Fatalf("bulk=%d %v", n, err)
	}
	unread, _, _ := s.ListArticles(ArticleQuery{Unread: true})
	if len(unread) != 3 || unread[0].FeedID != b.ID {
		t.Fatal("scope leaked")
	}
	if _, err = s.SaveGroup(g.ID, "阅读"); err != nil {
		t.Fatal(err)
	}
	reopened, _ := OpenStore(path)
	restored := NewService(reopened, fakeFetcher{})
	if restored.ListGroups()[0].Name != "阅读" {
		t.Fatal("group not persisted")
	}
	if err = restored.DeleteGroup(g.ID); err != nil {
		t.Fatal(err)
	}
	feeds, _ := restored.ListFeeds()
	if len(feeds) != 2 {
		t.Fatal("deleted subscriptions")
	}
	for _, f := range feeds {
		if f.GroupID != "" {
			t.Fatal("not ungrouped")
		}
	}
}

func TestSettingsMigrationAndValidation(t *testing.T) {
	store, _ := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	s := NewService(store, fakeFetcher{})
	settings := s.GetSettings()
	if settings.TimeoutSeconds != 30 || settings.Browser != "default" {
		t.Fatalf("bad defaults: %+v", settings)
	}
	settings.TimeoutSeconds = 0
	if s.SaveSettings(settings) == nil {
		t.Fatal("accepted invalid timeout")
	}
	settings = s.GetSettings()
	settings.Browser = "cmd.exe"
	if s.SaveSettings(settings) == nil {
		t.Fatal("accepted arbitrary executable")
	}
	settings = s.GetSettings()
	settings.DisplayMode = "rich"
	settings.ShowImages = true
	if err := s.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	reopened, _ := OpenStore(store.path)
	if !NewService(reopened, fakeFetcher{}).GetSettings().ShowImages {
		t.Fatal("settings lost")
	}
}

func TestBrowserURLValidation(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "file:///c:/test", "https://user:pass@example.com", "https://example.com/\n"} {
		if ValidateBrowserURL(u) == nil {
			t.Fatalf("accepted %q", u)
		}
	}
	if err := ValidateBrowserURL("https://example.com/article?a=1&b=2"); err != nil {
		t.Fatal(err)
	}
}
