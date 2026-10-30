package crawler

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/repository"
)

type CrawlResultData struct {
	IsSSR      bool
	StartedAt  time.Time
	FinishedAt time.Time
	Internal   []domain.LinkResult
	External   []domain.LinkResult
}

type SingleFlightManager struct {
	group        singleflight.Group
	mu           sync.RWMutex
	activeStores map[string]*repository.SafeMemoryStorage
	fetcher      *Fetcher
}

func NewSingleFlightManager() *SingleFlightManager {
	return &SingleFlightManager{
		activeStores: make(map[string]*repository.SafeMemoryStorage),
		fetcher:      NewFetcher(10 * time.Second),
	}
}

func NormalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	lower := strings.ToLower(rawURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}

func (m *SingleFlightManager) ExecuteOrJoin(rawURL string, workerCount int, maxPages int) (*repository.SafeMemoryStorage, func(ctx context.Context) (CrawlResultData, error)) {
	cleanURL := NormalizeURL(rawURL)
	flightKey := fmt.Sprintf("%s|pages:%d", cleanURL, maxPages)

	m.mu.Lock()
	store, exists := m.activeStores[flightKey]
	if !exists {
		store = repository.NewMemoryStorage()
		m.activeStores[flightKey] = store
	}
	m.mu.Unlock()

	runFunc := func(ctx context.Context) (CrawlResultData, error) {
		res, err, _ := m.group.Do(flightKey, func() (any, error) {
			defer func() {
				m.mu.Lock()
				delete(m.activeStores, flightKey)
				m.mu.Unlock()
			}()

			engine := NewEngine(store, workerCount, maxPages, m.fetcher)
			isSSR, startedAt, finishedAt := engine.Start(ctx, cleanURL)
			internal, external := store.GetResults()

			return CrawlResultData{
				IsSSR:      isSSR,
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
				Internal:   internal,
				External:   external,
			}, nil
		})
		if err != nil {
			return CrawlResultData{}, err
		}
		return res.(CrawlResultData), nil
	}
	return store, runFunc
}
