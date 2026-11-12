package rss

import (
	"strings"
	"testing"
	"time"
)

func TestRSSImageAttachments(t *testing.T) {
	input := `<rss xmlns:media="http://search.yahoo.com/mrss/" version="2.0"><channel><title>Images</title><item><guid>a</guid><enclosure url="/cover.jpg" type="image/jpeg"/></item><item><guid>b</guid><media:content url="https://example.com/pic.png" medium="image"/></item><item><guid>c</guid><enclosure url="/audio.mp3" type="audio/mpeg"/></item></channel></rss>`
	feed, err := Parse(strings.NewReader(input), "https://example.com/rss")
	if err != nil {
		t.Fatal(err)
	}
	if feed.Items[0].ImageURL != "https://example.com/cover.jpg" || feed.Items[1].ImageURL != "https://example.com/pic.png" || feed.Items[2].ImageURL != "" {
		t.Fatalf("bad images: %+v", feed.Items)
	}
}
func TestAtomXHTMLAndImageEnclosure(t *testing.T) {
	input := `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>a</id><content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Photo</p><img src="/photo.jpg"/></div></content><link rel="enclosure" type="image/png" href="/cover.png"/></entry></feed>`
	feed, err := Parse(strings.NewReader(input), "https://example.com/rss")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(feed.Items[0].Content, "photo.jpg") || feed.Items[0].ImageURL != "https://example.com/cover.png" {
		t.Fatalf("lost image: %+v", feed.Items[0])
	}
}

func TestParseRSSFeed(t *testing.T) {
	input := `<?xml version="1.0"?><rss version="2.0"><channel><title>Example</title><link>https://example.com</link><item><guid>post-1</guid><title>First post</title><link>https://example.com/1</link><description><![CDATA[<p>Hello</p>]]></description><pubDate>Mon, 02 Jan 2006 15:04:05 GMT</pubDate></item></channel></rss>`

	feed, err := Parse(strings.NewReader(input), "https://example.com/feed.xml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if feed.Title != "Example" || len(feed.Items) != 1 {
		t.Fatalf("unexpected feed: %#v", feed)
	}
	item := feed.Items[0]
	if item.ID == "" || item.Title != "First post" || item.Link != "https://example.com/1" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if item.PublishedAt.IsZero() || !item.PublishedAt.Equal(time.Date(2006, 1, 2, 15, 4, 5, 0, time.UTC)) {
		t.Fatalf("unexpected published time: %v", item.PublishedAt)
	}
}

func TestParseAtomFeed(t *testing.T) {
	input := `<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom feed</title><link href="https://example.com/"/><entry><id>tag:example.com,2025:1</id><title>Atom post</title><updated>2025-02-03T10:00:00Z</updated><link rel="alternate" href="https://example.com/post"/><summary>Summary</summary></entry></feed>`

	feed, err := Parse(strings.NewReader(input), "https://example.com/atom.xml")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if feed.Title != "Atom feed" || len(feed.Items) != 1 || feed.Items[0].Link != "https://example.com/post" {
		t.Fatalf("unexpected feed: %#v", feed)
	}
}

func TestParseRDFFeed(t *testing.T) {
	input := `<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/"><channel><title>RDF feed</title><link>https://example.com/</link></channel><item rdf:about="https://example.com/1"><title>RDF post</title><link>https://example.com/1</link><description>RDF summary</description></item></rdf:RDF>`
	feed, err := Parse(strings.NewReader(input), "https://example.com/feed.rdf")
	if err != nil || feed.Title != "RDF feed" || len(feed.Items) != 1 || feed.Items[0].Title != "RDF post" {
		t.Fatalf("unexpected RDF feed: %#v, %v", feed, err)
	}
}

func TestParseRejectsUnsupportedDocument(t *testing.T) {
	if _, err := Parse(strings.NewReader(`<html>not a feed</html>`), "https://example.com/feed"); err == nil {
		t.Fatal("Parse() accepted an HTML document")
	}
}

func TestValidateFeedURLRejectsLocalAndNonHTTPAddresses(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "http://127.0.0.1/feed.xml", "https://localhost/feed.xml", "https://[::1]/feed.xml"} {
		if err := ValidateFeedURL(raw); err == nil {
			t.Errorf("ValidateFeedURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestExtractArticleContentPrefersArticleAndRemovesNavigation(t *testing.T) {
	html := `<html><body><nav>Navigation</nav><main><article><h1>Story</h1><p>Full article text.</p><img src="/image.jpg"></article></main><footer>Footer</footer></body></html>`
	content, err := ExtractArticleContent(strings.NewReader(html), "https://example.com/story")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "Full article text.") || !strings.Contains(content, "https://example.com/image.jpg") || strings.Contains(content, "Navigation") || strings.Contains(content, "Footer") {
		t.Fatalf("unexpected extracted content: %s", content)
	}
}

func TestExtractArticleContentRejectsPagesWithoutReadableContent(t *testing.T) {
	if _, err := ExtractArticleContent(strings.NewReader(`<html><body><nav>Only navigation</nav></body></html>`), "https://example.com/story"); err == nil {
		t.Fatal("expected empty article error")
	}
}
