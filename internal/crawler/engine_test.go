package crawler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/repository"
)

func TestEngine_CrawlLevelsAndLinks(t *testing.T) {
	sitePages := map[string]string{
		"https://mysite.com": `
			<html><body>
				<a href="/page1">Page 1</a>
				<a href="/page2">Page 2</a>
				<a href="https://external.com/docs">External Docs</a>
			</body></html>`,
		"https://mysite.com/page1": `
			<html><body>
				<a href="/page3">Page 3</a>
			</body></html>`,
		"https://mysite.com/page2": `
			<html><body>
				<a href="/page1">Back to Page 1</a>
			</body></html>`,
		"https://mysite.com/page3": `
			<html><body>
				<p>Leaf page without links</p>
			</body></html>`,
		"https://external.com/docs": `<html><body>External site</body></html>`,
	}

	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		urlStr := req.URL.String()
		body, exists := sitePages[urlStr]
		if !exists {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString("not found")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}

		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(bytes.NewBufferString(body)),
			Header:        make(http.Header),
			Request:       req,
			ContentLength: int64(len(body)),
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	store := repository.NewMemoryStorage()
	engine := NewEngine(store, 4, 100, fetcher)

	ctx := context.Background()
	_, startedAt, finishedAt := engine.Start(ctx, "https://mysite.com")

	if finishedAt.Before(startedAt) {
		t.Error("expected finishedAt to be after startedAt")
	}

	progress := store.GetProgress()
	if !progress.IsFinished {
		t.Error("expected crawl to be marked as finished")
	}

	internal, external := store.GetResults()
	if len(internal) != 4 {
		t.Errorf("expected 4 internal links (root, page1, page2, page3), got %d", len(internal))
	}
	if len(external) != 1 {
		t.Errorf("expected 1 external link, got %d", len(external))
	}

	for _, link := range internal {
		if link.URL == "https://mysite.com" && link.DepthLevel != 1 {
			t.Errorf("expected root page depth level 1, got %d", link.DepthLevel)
		}
		if (link.URL == "https://mysite.com/page1" || link.URL == "https://mysite.com/page2") && link.DepthLevel != 2 {
			t.Errorf("expected level 2 for %s, got %d", link.URL, link.DepthLevel)
		}
		if link.URL == "https://mysite.com/page3" && link.DepthLevel != 3 {
			t.Errorf("expected level 3 for %s, got %d", link.URL, link.DepthLevel)
		}
	}
}

func TestEngine_MaxPagesLimit(t *testing.T) {
	sitePages := make(map[string]string)
	var rootLinks string
	for i := 1; i <= 20; i++ {
		pageURL := fmt.Sprintf("https://limit-test.com/page%d", i)
		rootLinks += fmt.Sprintf(`<a href="%s">Page %d</a>`, pageURL, i)
		sitePages[pageURL] = fmt.Sprintf("<html><body>Page %d</body></html>", i)
	}
	sitePages["https://limit-test.com"] = fmt.Sprintf("<html><body>%s</body></html>", rootLinks)

	var fetchedCount int
	var countMu sync.Mutex

	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		countMu.Lock()
		fetchedCount++
		countMu.Unlock()

		body := sitePages[req.URL.String()]
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	store := repository.NewMemoryStorage()
	maxPagesLimit := 5
	engine := NewEngine(store, 2, maxPagesLimit, fetcher)

	engine.Start(context.Background(), "https://limit-test.com")

	countMu.Lock()
	defer countMu.Unlock()
	if fetchedCount > maxPagesLimit {
		t.Errorf("expected at most %d pages fetched, got %d", maxPagesLimit, fetchedCount)
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		time.Sleep(50 * time.Millisecond)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("<html><body><a href=\"/p2\">P2</a></body></html>")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	store := repository.NewMemoryStorage()
	engine := NewEngine(store, 2, 100, fetcher)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(10*time.Millisecond, cancel)

	doneChan := make(chan struct{})
	go func() {
		engine.Start(ctx, "https://cancel-test.com")
		close(doneChan)
	}()

	select {
	case <-doneChan:
	case <-time.After(2 * time.Second):
		t.Fatal("engine did not stop on context cancellation within 2s timeout")
	}
}
