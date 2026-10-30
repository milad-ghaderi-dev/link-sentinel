package crawler

import (
	"io"
	"net/url"
	"path"
	"strings"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"golang.org/x/net/html"
)

var knownFileExtensions = map[string]string{
	".pdf":  "pdf",
	".png":  "png",
	".jpg":  "jpg",
	".jpeg": "jpeg",
	".gif":  "gif",
	".zip":  "zip",
	".rar":  "rar",
	".svg":  "svg",
	".mp4":  "mp4",
	".mp3":  "mp3",
	".docx": "docx",
	".webp": "webp",
	".xlsx": "xlsx",
}

func DetectFileType(u string) (string, bool) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", false
	}
	return detectFileTypeFromPath(parsed.Path)
}

func detectFileTypeFromPath(rawPath string) (string, bool) {
	ext := strings.ToLower(path.Ext(rawPath))
	if fileType, exists := knownFileExtensions[ext]; exists {
		return fileType, true
	}
	return "", false
}

func ExtractDetailedLinks(baseURLStr string, body io.Reader) ([]domain.LinkResult, error) {
	var results []domain.LinkResult
	baseURL, err := url.Parse(baseURLStr)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(body)
	if err != nil {
		return nil, err
	}

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			var rawHref string
			var isNoFollow bool
			hasHref := false

			for _, attr := range n.Attr {
				if attr.Key == "href" {
					hasHref = true
					rawHref = strings.TrimSpace(attr.Val)
				}

				if attr.Key == "rel" && strings.Contains(strings.ToLower(attr.Val), "nofollow") {
					isNoFollow = true
				}
			}

			anchorText := extractNodeText(n)
			linkInfo := domain.LinkResult{
				AnchorText: anchorText,
				IsNoFollow: isNoFollow,
				ParentURL:  baseURLStr,
			}

			if !hasHref {
				linkInfo.CustomStatus = domain.StatusMissingHref
				results = append(results, linkInfo)
			} else if rawHref == "" {
				linkInfo.CustomStatus = domain.StatusEmptyHref
				results = append(results, linkInfo)
			} else if strings.HasPrefix(rawHref, "mailto:") || strings.HasPrefix(rawHref, "tel:") {
				linkInfo.URL = rawHref
				linkInfo.CustomStatus = domain.StatusSpecialProtocol
				results = append(results, linkInfo)
			} else if strings.HasPrefix(rawHref, "#") || strings.HasPrefix(rawHref, "javascript:") {
			} else {
				relURL, err := url.Parse(rawHref)
				if err != nil {
					linkInfo.URL = rawHref
					linkInfo.CustomStatus = domain.StatusInvalidProtocol
					results = append(results, linkInfo)
				} else {
					resolvedURL := baseURL.ResolveReference(relURL)
					resolvedURL.Fragment = ""
					resolvedURLStr := resolvedURL.String()
					linkInfo.URL = resolvedURLStr

					if fileExt, isFile := detectFileTypeFromPath(resolvedURL.Path); isFile {
						linkInfo.CustomStatus = domain.StatusFileAsset
						linkInfo.FileType = fileExt
					}
					results = append(results, linkInfo)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)
	return results, nil
}

func extractNodeText(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			sb.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(sb.String())
}
