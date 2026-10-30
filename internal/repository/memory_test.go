package repository

import (
	"fmt"
	"sync"
	"testing"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

func TestSafeMemoryStorage_ConcurrentAccess(t *testing.T) {
	store := NewMemoryStorage()
	var wg sync.WaitGroup
	workers := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			url := fmt.Sprintf("https://example.com/page/%d", workerID)
			store.AddInternal(domain.LinkResult{
				URL:        url,
				DepthLevel: 1,
			})
			store.UpdateInternalDetails(url, domain.LinkResult{
				URL:        url,
				HTTPStatus: 200,
			})
			store.AddExternal(fmt.Sprintf("https://external.com/%d", workerID), domain.LinkResult{
				URL:        fmt.Sprintf("https://external.com/%d", workerID),
				HTTPStatus: 200,
			})
			_ = store.GetProgress()
		}(i)
	}

	wg.Wait()
	store.MarkFinished()

	progress := store.GetProgress()
	if progress.InternalFound != workers {
		t.Fatalf("expected %d internal links, got %d", workers, progress.InternalFound)
	}
	if progress.ExternalFound != workers {
		t.Fatalf("expected %d external links, got %d", workers, progress.ExternalFound)
	}
	if !progress.IsFinished {
		t.Fatal("expected crawl progress to be marked as finished")
	}
}

func TestSafeMemoryStorage_AddInternal_Duplicates(t *testing.T) {
	store := NewMemoryStorage()

	first := store.AddInternal(domain.LinkResult{URL: "https://example.com/page1"})
	if !first {
		t.Fatal("expected first AddInternal to return true")
	}

	second := store.AddInternal(domain.LinkResult{URL: "https://example.com/page1"})
	if second {
		t.Fatal("expected second AddInternal with duplicate URL to return false")
	}
}

func TestSafeMemoryStorage_AddExternal_Duplicates(t *testing.T) {
	store := NewMemoryStorage()

	first := store.AddExternal("https://external.com", domain.LinkResult{URL: "https://external.com"})
	if !first {
		t.Fatal("expected first AddExternal to return true")
	}

	second := store.AddExternal("https://external.com", domain.LinkResult{URL: "https://external.com"})
	if second {
		t.Fatal("expected second AddExternal with duplicate URL to return false")
	}
}

func TestSafeMemoryStorage_UpdateInternalDetails_AttributePreservation(t *testing.T) {
	store := NewMemoryStorage()

	store.AddInternal(domain.LinkResult{
		URL:        "https://example.com/item",
		AnchorText: "Initial Anchor",
		IsNoFollow: true,
		FileType:   "text/html",
		ParentURL:  "https://example.com",
		DepthLevel: 2,
	})

	// Update with only HTTP status and custom status; other fields empty
	store.UpdateInternalDetails("https://example.com/item", domain.LinkResult{
		URL:          "https://example.com/item",
		HTTPStatus:   200,
		CustomStatus: domain.StatusOK,
	})

	internal, _ := store.GetResults()
	if len(internal) != 1 {
		t.Fatalf("expected 1 internal link, got %d", len(internal))
	}

	item := internal[0]
	if item.AnchorText != "Initial Anchor" {
		t.Errorf("expected AnchorText 'Initial Anchor' preserved, got '%s'", item.AnchorText)
	}
	if !item.IsNoFollow {
		t.Errorf("expected IsNoFollow true preserved, got false")
	}
	if item.FileType != "text/html" {
		t.Errorf("expected FileType 'text/html' preserved, got '%s'", item.FileType)
	}
	if item.ParentURL != "https://example.com" {
		t.Errorf("expected ParentURL preserved, got '%s'", item.ParentURL)
	}
	if item.DepthLevel != 2 {
		t.Errorf("expected DepthLevel 2 preserved, got %d", item.DepthLevel)
	}
	if item.HTTPStatus != 200 {
		t.Errorf("expected HTTPStatus 200, got %d", item.HTTPStatus)
	}
}

func TestSafeMemoryStorage_GetResults(t *testing.T) {
	store := NewMemoryStorage()

	store.AddInternal(domain.LinkResult{URL: "https://example.com/a"})
	store.AddInternal(domain.LinkResult{URL: "https://example.com/b"})
	store.AddExternal("https://ext.com/1", domain.LinkResult{URL: "https://ext.com/1"})

	internal, external := store.GetResults()
	if len(internal) != 2 {
		t.Fatalf("expected 2 internal links, got %d", len(internal))
	}
	if len(external) != 1 {
		t.Fatalf("expected 1 external link, got %d", len(external))
	}
}
