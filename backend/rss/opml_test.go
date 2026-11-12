package rss

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOPMLRoundTripAndImportDedup(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><opml version="2.0"><body><outline text="技术"><outline type="rss" title="Example" xmlUrl="https://example.com/feed"/></outline><outline type="rss" xmlUrl="https://example.org/feed"/></body></opml>`)
	feeds, err := ParseOPML(data)
	if err != nil || len(feeds) != 2 || feeds[0].Group != "技术" {
		t.Fatalf("parse=%+v err=%v", feeds, err)
	}
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(store, fakeFetcher{feed: Feed{Title: "Imported"}})
	r, err := s.ImportOPML(context.Background(), data)
	if err != nil || len(r.Added) != 2 {
		t.Fatalf("import=%+v err=%v", r, err)
	}
	r, _ = s.ImportOPML(context.Background(), data)
	if len(r.Skipped) != 2 {
		t.Fatalf("dedup=%+v", r)
	}
	out, _ := s.ExportOPML(nil)
	if !strings.Contains(out, "xmlUrl=\"https://example.com/feed\"") {
		t.Fatal("export lost feed")
	}
}
