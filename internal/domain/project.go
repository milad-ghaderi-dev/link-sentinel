package domain

import "time"

type Project struct {
	ID              int        `json:"id"`
	UserID          int        `json:"user_id"`
	Name            string     `json:"name"`
	StartURL        string     `json:"start_url"`
	IsSSR           bool       `json:"is_ssr"`
	Status          string     `json:"status"`
	CrawlStartedAt  *time.Time `json:"crawl_started_at"`
	CrawlFinishedAt *time.Time `json:"crawl_finished_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

type LinkHierarchyNode struct {
	ParentURL string       `json:"parent_url"`
	Links     []LinkResult `json:"links"`
}
