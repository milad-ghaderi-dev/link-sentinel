package repository

import (
	"sync"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

type SafeMemoryStorage struct {
	mu            sync.RWMutex
	internalLinks map[string]domain.LinkResult
	externalLinks map[string]domain.LinkResult
	isFinished    bool
}

func NewMemoryStorage() *SafeMemoryStorage {
	return &SafeMemoryStorage{
		internalLinks: make(map[string]domain.LinkResult),
		externalLinks: make(map[string]domain.LinkResult),
	}
}

func (s *SafeMemoryStorage) AddInternal(item domain.LinkResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.internalLinks[item.URL]; exists {
		return false
	}
	s.internalLinks[item.URL] = item
	return true
}

func (s *SafeMemoryStorage) UpdateInternalDetails(url string, item domain.LinkResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, exists := s.internalLinks[url]; exists {
		if item.AnchorText == "" {
			item.AnchorText = existing.AnchorText
		}
		if !item.IsNoFollow {
			item.IsNoFollow = existing.IsNoFollow
		}
		if item.FileType == "" {
			item.FileType = existing.FileType
		}
		if item.ParentURL == "" {
			item.ParentURL = existing.ParentURL
		}
		if item.DepthLevel == 0 {
			item.DepthLevel = existing.DepthLevel
		}
	}

	s.internalLinks[url] = item
}

func (s *SafeMemoryStorage) AddExternal(url string, item domain.LinkResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.externalLinks[url]; exists {
		return false
	}
	s.externalLinks[url] = item
	return true
}

func (s *SafeMemoryStorage) UpdateExternal(url string, item domain.LinkResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.externalLinks[url] = item
}

func (s *SafeMemoryStorage) MarkFinished() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isFinished = true
}

func (s *SafeMemoryStorage) GetProgress() domain.CrawlProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	inCount := len(s.internalLinks)
	exCount := len(s.externalLinks)

	return domain.CrawlProgress{
		TotalFound:    inCount + exCount,
		InternalFound: inCount,
		ExternalFound: exCount,
		IsFinished:    s.isFinished,
	}
}

func (s *SafeMemoryStorage) GetResults() ([]domain.LinkResult, []domain.LinkResult) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	internal := make([]domain.LinkResult, 0, len(s.internalLinks))
	for _, val := range s.internalLinks {
		internal = append(internal, val)
	}

	external := make([]domain.LinkResult, 0, len(s.externalLinks))
	for _, val := range s.externalLinks {
		external = append(external, val)
	}

	return internal, external
}
