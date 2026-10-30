package domain

type LinkStatus int

const (
	StatusOK              LinkStatus = 0
	StatusEmptyContent    LinkStatus = 1
	StatusMixedContent    LinkStatus = 4
	StatusMissingHref     LinkStatus = 5
	StatusEmptyHref       LinkStatus = 6
	StatusSpecialProtocol LinkStatus = 7
	StatusInvalidProtocol LinkStatus = 10
	StatusBrokenLink      LinkStatus = 14
	StatusFileAsset       LinkStatus = 15
	StatusDuplicateURL    LinkStatus = 16
	StatusForbidden       LinkStatus = 17
)

func (s LinkStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusEmptyContent:
		return "Empty Content"
	case StatusMixedContent:
		return "Mixed Content"
	case StatusMissingHref:
		return "Missing Href"
	case StatusEmptyHref:
		return "Empty Href"
	case StatusSpecialProtocol:
		return "Special Protocol"
	case StatusInvalidProtocol:
		return "Invalid Protocol"
	case StatusBrokenLink:
		return "Broken Link"
	case StatusFileAsset:
		return "File Asset"
	case StatusDuplicateURL:
		return "Duplicate URL"
	case StatusForbidden:
		return "Forbidden"
	default:
		return "Unknown"
	}
}

type LinkResult struct {
	URL           string     `json:"url"`
	AnchorText    string     `json:"anchor_text"`
	FileType      string     `json:"file_type,omitempty"`
	ParentURL     string     `json:"parent_url,omitempty"`
	RedirectChain []string   `json:"redirect_chain,omitempty"`
	CustomStatus  LinkStatus `json:"custom_status"`
	HTTPStatus    int        `json:"http_status"`
	DepthLevel    int        `json:"depth_level"`
	IsNoFollow    bool       `json:"is_no_follow"`
}

type CrawlProgress struct {
	TotalFound    int  `json:"total_found"`
	InternalFound int  `json:"internal_found"`
	ExternalFound int  `json:"external_found"`
	IsFinished    bool `json:"is_finished"`
}

type LinkSearchFilter struct {
	HTTPStatusMin  *int         `json:"http_status_min"`
	HTTPStatusMax  *int         `json:"http_status_max"`
	IsNoFollow     *bool        `json:"is_nofollow"`
	LinkType       string       `json:"link_type"`
	ParentURL      string       `json:"parent_url"`
	URLContains    string       `json:"url_contains"`
	URLStartsWith  string       `json:"url_starts_with"`
	URLEndsWith    string       `json:"url_ends_with"`
	AnchorContains string       `json:"anchor_contains"`
	CustomStatuses []LinkStatus `json:"custom_statuses"`
	HTTPStatuses   []int        `json:"http_statuses"`
	DepthLevels    []int        `json:"depth_levels"`
	Limit          int          `json:"limit"`
	Offset         int          `json:"offset"`
}

type SearchResultMeta struct {
	TotalLinks    int `json:"total_links"`
	InternalLinks int `json:"internal_links"`
	ExternalLinks int `json:"external_links"`
	Limit         int `json:"limit"`
	Offset        int `json:"offset"`
}
