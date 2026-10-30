package crawler

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

type Fetcher struct {
	client *http.Client
}

type FetchPageResult struct {
	FinalURL      string
	StatusCode    int
	Body          []byte
	RedirectChain []string
	CustomStatus  domain.LinkStatus
}

type redirectKey struct{}

func NewFetcher(timeout time.Duration, customTransport ...http.RoundTripper) *Fetcher {
	var tr http.RoundTripper
	if len(customTransport) > 0 && customTransport[0] != nil {
		tr = customTransport[0]
	} else {
		tr = &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if chain, ok := req.Context().Value(redirectKey{}).(*[]string); ok {
				*chain = append(*chain, req.URL.String())
			}
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	return &Fetcher{
		client: client,
	}
}

func (f *Fetcher) FetchPage(targetURL string) (*FetchPageResult, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return &FetchPageResult{FinalURL: targetURL, CustomStatus: domain.StatusBrokenLink}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")

	var chain []string
	ctx := context.WithValue(req.Context(), redirectKey{}, &chain)
	req = req.WithContext(ctx)

	resp, err := f.client.Do(req)
	if err != nil {
		return &FetchPageResult{FinalURL: targetURL, CustomStatus: domain.StatusBrokenLink}, err
	}
	defer resp.Body.Close()

	var body []byte
	if resp.StatusCode < 400 {
		body, _ = io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	} else {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	}

	customStatus := domain.StatusOK
	if resp.StatusCode == 403 {
		customStatus = domain.StatusForbidden
	} else if resp.StatusCode >= 400 {
		customStatus = domain.StatusBrokenLink
	} else if len(body) == 0 && resp.StatusCode == 200 {
		customStatus = domain.StatusEmptyContent
	}

	return &FetchPageResult{
		FinalURL:      resp.Request.URL.String(),
		StatusCode:    resp.StatusCode,
		Body:          body,
		RedirectChain: chain,
		CustomStatus:  customStatus,
	}, nil
}

func (f *Fetcher) CheckExternalLinkWithChain(targetURL string) (int, []string, domain.LinkStatus) {
	req, err := http.NewRequest("HEAD", targetURL, nil)
	if err != nil {
		return 0, nil, domain.StatusBrokenLink
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")

	var chain []string
	ctx := context.WithValue(req.Context(), redirectKey{}, &chain)
	req = req.WithContext(ctx)

	resp, err := f.client.Do(req)
	if err == nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotImplemented) {
		if resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
		}
		req.Method = "GET"
		resp, err = f.client.Do(req)
	}

	if err != nil {
		return 0, nil, domain.StatusBrokenLink
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
		}
	}()

	customStatus := domain.StatusOK
	if resp.StatusCode == 403 {
		customStatus = domain.StatusForbidden
	} else if resp.StatusCode >= 400 {
		customStatus = domain.StatusBrokenLink
	}
	return resp.StatusCode, chain, customStatus
}

func CheckIsSSR(bodyContent string) bool {
	trimmed := strings.TrimSpace(bodyContent)
	if len(trimmed) < 300 {
		return false
	}
	if strings.Contains(bodyContent, "<div id=\"root\"></div>") || strings.Contains(bodyContent, "<div id=\"app\"></div>") {
		return false
	}
	return true
}
