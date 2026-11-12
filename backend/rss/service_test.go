package rss

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type fakeFetcher struct {
	feed Feed
	err  error
}

func TestListFeedsReturnsEmptyArrayWhenNoSubscriptions(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	feeds, err := NewService(store, fakeFetcher{}).ListFeeds()
	if err != nil {
		t.Fatal(err)
	}
	if feeds == nil {
		t.Fatal("ListFeeds() returned nil; JSON would encode it as null instead of []")
	}
}

func (f fakeFetcher) Fetch(context.Context, string, string, string) (FetchedFeed, error) {
	return FetchedFeed{Feed: f.feed}, f.err
}

func TestRefreshPreservesReadStateAndDeduplicatesArticles(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "rss", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	first := Article{ID: "one", Title: "One", PublishedAt: time.Now().UTC()}
	service := NewService(store, fakeFetcher{feed: Feed{Title: "Example", Items: []Article{first}}})
	feed, err := service.AddFeed(context.Background(), "https://example.com/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetRead(feed.ID, first.ID, true); err != nil {
		t.Fatal(err)
	}

	service.fetcher = fakeFetcher{feed: Feed{Title: "Renamed", Items: []Article{first, {ID: "two", Title: "Two"}}}}
	if _, err := service.Refresh(context.Background(), feed.ID); err != nil {
		t.Fatal(err)
	}
	articles, total, err := service.ListArticles(ArticleQuery{FeedID: feed.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(articles) != 2 {
		t.Fatalf("got %d articles, total %d; want 2", len(articles), total)
	}
	if !articles[0].IsRead && !articles[1].IsRead {
		t.Fatal("refresh did not preserve the existing read state")
	}
	feeds, err := service.ListFeeds()
	if err != nil {
		t.Fatal(err)
	}
	if feeds[0].Title != "Renamed" || feeds[0].UnreadCount != 1 {
		t.Fatalf("unexpected feed after refresh: %#v", feeds[0])
	}
}

func TestAddFeedRejectsDuplicateAndKeepsFailedFeedOut(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, fakeFetcher{feed: Feed{Title: "Example"}})
	if _, err := service.AddFeed(context.Background(), "https://example.com/feed.xml"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddFeed(context.Background(), "https://example.com/feed.xml"); err == nil {
		t.Fatal("duplicate feed was accepted")
	}
	service.fetcher = fakeFetcher{err: errors.New("offline")}
	if _, err := service.AddFeed(context.Background(), "https://example.org/feed.xml"); err == nil {
		t.Fatal("failed subscription was accepted")
	}
	feeds, err := service.ListFeeds()
	if err != nil || len(feeds) != 1 {
		t.Fatalf("failed add changed subscriptions: feeds=%d err=%v", len(feeds), err)
	}
}

func TestStorePersistsAndDeletesSubscriptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rss", "state.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store, fakeFetcher{feed: Feed{Title: "Example"}})
	feed, err := service.AddFeed(context.Background(), "https://example.com/feed.xml")
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewService(reopened, fakeFetcher{}).ListFeeds()
	if err != nil || len(restored) != 1 || restored[0].ID != feed.ID {
		t.Fatalf("subscription was not persisted: %#v, %v", restored, err)
	}
	if err := NewService(reopened, fakeFetcher{}).DeleteFeed(feed.ID); err != nil {
		t.Fatal(err)
	}
	final, err := NewService(reopened, fakeFetcher{}).ListFeeds()
	if err != nil || len(final) != 0 {
		t.Fatalf("subscription was not deleted: %#v, %v", final, err)
	}
}

func TestDeleteFeedsRemovesSelectedFeedsAndArticles(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(store, fakeFetcher{feed: Feed{Items: []Article{{ID: "article", Title: "A"}}}})
	a, err := s.AddFeed(context.Background(), "https://example.com/a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddFeed(context.Background(), "https://example.com/b")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFeeds([]string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	feeds, _ := s.ListFeeds()
	articles, total, _ := s.ListArticles(ArticleQuery{Limit: 20})
	if len(feeds) != 0 || total != 0 || len(articles) != 0 {
		t.Fatalf("feeds=%d articles=%d total=%d", len(feeds), len(articles), total)
	}
}

func TestDeleteFeedsRejectsUnknownIDWithoutRemovingAnything(t *testing.T) {
	store, _ := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	s := NewService(store, fakeFetcher{feed: Feed{Title: "Example"}})
	feed, err := s.AddFeed(context.Background(), "https://example.com/feed")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFeeds([]string{feed.ID, "missing"}); err == nil {
		t.Fatal("expected unknown id error")
	}
	feeds, _ := s.ListFeeds()
	if len(feeds) != 1 || feeds[0].ID != feed.ID {
		t.Fatalf("unexpected feed state: %+v", feeds)
	}
}
