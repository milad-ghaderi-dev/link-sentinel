package crawler

import (
	"strings"
	"testing"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantExt    string
		wantIsFile bool
	}{
		{name: "pdf file", url: "https://example.com/docs/manual.pdf", wantExt: "pdf", wantIsFile: true},
		{name: "png image uppercase", url: "https://example.com/images/LOGO.PNG", wantExt: "png", wantIsFile: true},
		{name: "zip archive with query", url: "https://example.com/download.zip?v=1", wantExt: "zip", wantIsFile: true},
		{name: "normal web page", url: "https://example.com/blog/article", wantExt: "", wantIsFile: false},
		{name: "invalid url", url: "http://invalid-url-example/??", wantExt: "", wantIsFile: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExt, gotIsFile := DetectFileType(tt.url)
			if gotExt != tt.wantExt || gotIsFile != tt.wantIsFile {
				t.Fatalf("DetectFileType(%q) = (%q, %v), want (%q, %v)", tt.url, gotExt, gotIsFile, tt.wantExt, tt.wantIsFile)
			}
		})
	}
}

func TestExtractDetailedLinks(t *testing.T) {
	htmlData := `
	<!DOCTYPE html>
	<html>
	<body>
		<a href="/about">About <span>Us</span></a>
		<a href="https://external.com/privacy" rel="nofollow">Privacy</a>
		<a href="/downloads/report.pdf">Download Report</a>
		<a href="mailto:support@example.com">Contact Email</a>
		<a href="#top">Jump to top</a>
		<a href="javascript:void(0)">Click me</a>
		<a>No href link</a>
		<a href="">Empty href link</a>
	</body>
	</html>
	`

	baseURL := "https://example.com/home"
	links, err := ExtractDetailedLinks(baseURL, strings.NewReader(htmlData))
	if err != nil {
		t.Fatalf("unexpected error parsing links: %v", err)
	}

	expectedCount := 6
	if len(links) != expectedCount {
		t.Fatalf("expected %d links, got %d", expectedCount, len(links))
	}

	aboutLink := links[0]
	if aboutLink.URL != "https://example.com/about" {
		t.Fatalf("expected resolved URL https://example.com/about, got %s", aboutLink.URL)
	}
	if aboutLink.AnchorText != "About Us" {
		t.Fatalf("expected anchor text 'About Us', got %q", aboutLink.AnchorText)
	}

	privacyLink := links[1]
	if !privacyLink.IsNoFollow {
		t.Fatal("expected privacy link to be marked as nofollow")
	}

	pdfLink := links[2]
	if pdfLink.CustomStatus != domain.StatusFileAsset || pdfLink.FileType != "pdf" {
		t.Fatalf("expected PDF asset status, got %v with ext %s", pdfLink.CustomStatus, pdfLink.FileType)
	}

	mailLink := links[3]
	if mailLink.CustomStatus != domain.StatusSpecialProtocol {
		t.Fatalf("expected special protocol for mailto, got %v", mailLink.CustomStatus)
	}

	noHrefLink := links[4]
	if noHrefLink.CustomStatus != domain.StatusMissingHref {
		t.Fatalf("expected missing href status, got %v", noHrefLink.CustomStatus)
	}

	emptyHrefLink := links[5]
	if emptyHrefLink.CustomStatus != domain.StatusEmptyHref {
		t.Fatalf("expected empty href status, got %v", emptyHrefLink.CustomStatus)
	}
}

func TestExtractDetailedLinks_FragmentStripped(t *testing.T) {
	htmlData := `<a href="/docs/intro#getting-started">Docs</a>`
	links, err := ExtractDetailedLinks("https://example.com", strings.NewReader(htmlData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].URL != "https://example.com/docs/intro" {
		t.Fatalf("expected fragment to be stripped, got %s", links[0].URL)
	}
}
