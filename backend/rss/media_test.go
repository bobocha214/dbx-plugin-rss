package rss

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchImageReusesHTTPConnection(t *testing.T) {
	var reused atomic.Int32
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		if info.Reused {
			reused.Add(1)
		}
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()
	ctx := httptrace.WithClientTrace(context.Background(), trace)
	for range 2 {
		if _, err := fetchImageWithClient(ctx, "http://images.example.com/photo.png", client); err != nil {
			t.Fatal(err)
		}
	}
	if reused.Load() != 1 {
		t.Fatalf("expected second image request to reuse the HTTP connection, reused=%d", reused.Load())
	}
}

func TestImageRejectsLocalTargetsAndNonImages(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1/x", "http://localhost/x", "file:///x"} {
		if _, err := FetchImage(context.Background(), u); err == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	if _, err := imageDataURL([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")); err == nil {
		t.Fatal("accepted SVG")
	}
	if _, err := imageDataURL([]byte("<html>bad</html>")); err == nil {
		t.Fatal("accepted HTML")
	}
}

type deadlineFetcher struct{}

func (deadlineFetcher) Fetch(ctx context.Context, _ string, _ string, _ string) (FetchedFeed, error) {
	if _, ok := ctx.Deadline(); !ok {
		return FetchedFeed{}, &net.DNSError{Err: "missing deadline"}
	}
	return FetchedFeed{}, context.DeadlineExceeded
}
func TestFetchUsesConfiguredDeadline(t *testing.T) {
	store, _ := OpenStore(t.TempDir() + "/state.json")
	s := NewService(store, deadlineFetcher{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.AddFeed(ctx, "https://example.com/rss")
	if err == nil || !stringsContains(err.Error(), "超时") {
		t.Fatalf("missing timeout explanation: %v", err)
	}
}
func stringsContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
