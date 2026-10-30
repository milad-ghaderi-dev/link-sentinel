package crawler

import (
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trailing slash removed",
			input:    "https://example.com/about/",
			expected: "https://example.com/about",
		},
		{
			name:     "uppercase host to lowercase",
			input:    "HTTPS://EXAMPLE.COM/Page",
			expected: "https://example.com/Page",
		},
		{
			name:     "whitespaces trimmed",
			input:    "   https://example.com/contact   ",
			expected: "https://example.com/contact",
		},
		{
			name:     "root domain trailing slash removed",
			input:    "https://example.com/",
			expected: "https://example.com",
		},
		{
			name:     "missing scheme prepends https",
			input:    "example.com",
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeURL(tt.input)
			if result != tt.expected {
				t.Fatalf("NormalizeURL(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSingleFlightManager_CoalesceAndDistinct(t *testing.T) {
	mgr := NewSingleFlightManager()

	store1, _ := mgr.ExecuteOrJoin("https://test.com", 2, 50)
	store2, _ := mgr.ExecuteOrJoin("https://test.com", 2, 50)

	if store1 != store2 {
		t.Error("expected identical URL and maxPages to share the same SafeMemoryStorage")
	}

	store3, _ := mgr.ExecuteOrJoin("https://test.com", 2, 200)
	if store1 == store3 {
		t.Error("expected different maxPages limits to have distinct SafeMemoryStorage")
	}

	store4, _ := mgr.ExecuteOrJoin("https://other.com", 2, 50)
	if store1 == store4 {
		t.Error("expected different URLs to have distinct SafeMemoryStorage")
	}
}
