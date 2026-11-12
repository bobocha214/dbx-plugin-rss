package main

import (
	"encoding/json"
	"github.com/example/dbx-plugin-rss/rss"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryRPCUsesTrustedArticleSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"feeds":[{"id":"f","title":"Source","url":"https://example.com/rss"}],"articles":[{"feedId":"f","id":"a","title":"Original","content":"Trusted local content"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := rss.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	p := Plugin{service: rss.NewService(store, nil)}
	if _, e := p.libraryRPC("rss/library/setFavorite", json.RawMessage(`{"feedId":"f","articleId":"a","favorite":true,"content":"<script>bad</script>"}`)); e != nil {
		t.Fatal(e)
	}
	saved, err := p.service.GetArticle("f", "a")
	if err != nil || saved.Content != "Trusted local content" || !saved.IsFavorite {
		t.Fatalf("untrusted snapshot: %+v %v", saved, err)
	}
	for _, test := range []struct{ method, params string }{
		{"rss/library/saveAnnotation", `{"feedId":"f","articleId":"a","kind":"color","color":"url(evil)","quote":"Trusted","start":0,"end":7,"mode":"rich"}`},
		{"rss/library/setFavorite", `{"feedId":"missing","articleId":"a","favorite":true}`},
		{"rss/library/saveCategory", `{broken`},
	} {
		if _, e := p.libraryRPC(test.method, json.RawMessage(test.params)); e == nil {
			t.Fatalf("accepted %s", test.params)
		}
	}
}
