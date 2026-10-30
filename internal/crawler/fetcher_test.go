package crawler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetcher_FetchPage_Success200(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		html := "<html><body><h1>Hello World</h1></body></html>"
		return &http.Response{
			StatusCode:    http.StatusOK,
			Body:          io.NopCloser(bytes.NewBufferString(html)),
			Header:        make(http.Header),
			Request:       req,
			ContentLength: int64(len(html)),
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	res, err := fetcher.FetchPage("https://example.com/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.StatusCode != 200 {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}
	if res.CustomStatus != domain.StatusOK {
		t.Errorf("expected StatusOK, got %v", res.CustomStatus)
	}
	if !bytes.Contains(res.Body, []byte("Hello World")) {
		t.Errorf("expected body to contain Hello World, got %s", string(res.Body))
	}
}

func TestFetcher_FetchPage_Forbidden403(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(bytes.NewBufferString("forbidden")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	res, err := fetcher.FetchPage("https://example.com/forbidden")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StatusCode != 403 {
		t.Errorf("expected 403, got %d", res.StatusCode)
	}
	if res.CustomStatus != domain.StatusForbidden {
		t.Errorf("expected StatusForbidden, got %v", res.CustomStatus)
	}
}

func TestFetcher_FetchPage_BrokenLink500(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString("server error")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	res, err := fetcher.FetchPage("https://example.com/error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StatusCode != 500 {
		t.Errorf("expected 500, got %d", res.StatusCode)
	}
	if res.CustomStatus != domain.StatusBrokenLink {
		t.Errorf("expected StatusBrokenLink, got %v", res.CustomStatus)
	}
}

func TestFetcher_FetchPage_EmptyContent(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	fetcher := NewFetcher(5*time.Second, tr)
	res, err := fetcher.FetchPage("https://example.com/empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CustomStatus != domain.StatusEmptyContent {
		t.Errorf("expected StatusEmptyContent, got %v", res.CustomStatus)
	}
}

func TestFetcher_FetchPage_NetworkError(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset by peer")
	})

	fetcher := NewFetcher(5*time.Second, tr)
	res, err := fetcher.FetchPage("https://example.com/down")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if res.CustomStatus != domain.StatusBrokenLink {
		t.Errorf("expected StatusBrokenLink on network error, got %v", res.CustomStatus)
	}
}

func TestFetcher_CheckExternalLinkWithChain_MethodNotAllowed_FallbackGET(t *testing.T) {
	headCalled := false
	getCalled := false

	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == "HEAD" {
			headCalled = true
			return &http.Response{
				StatusCode: http.StatusMethodNotAllowed,
				Body:       io.NopCloser(bytes.NewBufferString("method not allowed")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		if req.Method == "GET" {
			getCalled = true
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("ok")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}
		return nil, errors.New("unknown method")
	})

	fetcher := NewFetcher(5*time.Second, tr)
	status, _, custom := fetcher.CheckExternalLinkWithChain("https://external.com/api")
	if !headCalled {
		t.Error("expected HEAD request first")
	}
	if !getCalled {
		t.Error("expected fallback GET request after 405")
	}
	if status != 200 {
		t.Errorf("expected final status 200, got %d", status)
	}
	if custom != domain.StatusOK {
		t.Errorf("expected StatusOK, got %v", custom)
	}
}

func TestFetcher_CheckExternalLinkWithChain_NetworkError(t *testing.T) {
	tr := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial timeout")
	})

	fetcher := NewFetcher(5*time.Second, tr)
	status, _, custom := fetcher.CheckExternalLinkWithChain("https://deadlink.com")
	if status != 0 {
		t.Errorf("expected status 0, got %d", status)
	}
	if custom != domain.StatusBrokenLink {
		t.Errorf("expected StatusBrokenLink, got %v", custom)
	}
}

func TestIsSameHost(t *testing.T) {
	tests := []struct {
		hostA string
		hostB string
		want  bool
	}{
		{"example.com", "www.example.com", true},
		{"www.example.com", "example.com", true},
		{"example.com:8080", "example.com", true},
		{"go.dev", "go.dev", true},
		{"sub.example.com", "example.com", false},
		{"google.com", "github.com", false},
		{"", "", false},
	}

	for _, tt := range tests {
		got := isSameHost(tt.hostA, tt.hostB)
		if got != tt.want {
			t.Errorf("isSameHost(%q, %q) = %v, want %v", tt.hostA, tt.hostB, got, tt.want)
		}
	}
}

func TestCheckIsSSR(t *testing.T) {
	shortContent := "<div>Too short</div>"
	if CheckIsSSR(shortContent) {
		t.Error("expected short content to return false for SSR")
	}

	spaContent := "<html><body><div id=\"root\"></div><script src=\"app.js\"></script>" + string(make([]byte, 400)) + "</body></html>"
	if CheckIsSSR(spaContent) {
		t.Error("expected SPA with empty #root to return false for SSR")
	}

	ssrContent := "<html><head><title>SSR Page</title></head><body><article><h1>Deep Content</h1><p>Full server rendered article...</p></article>" + string(make([]byte, 400)) + "</body></html>"
	if !CheckIsSSR(ssrContent) {
		t.Error("expected full SSR content to return true")
	}
}
