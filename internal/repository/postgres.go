package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

type PostgresStorage struct {
	Pool *pgxpool.Pool
}

func NewPostgresStorage(ctx context.Context, connStr string) (*PostgresStorage, error) {
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &PostgresStorage{Pool: pool}
	if err := store.migrate(ctx); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

func (p *PostgresStorage) migrate(ctx context.Context) error {
	queryUsers := `
	CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);`

	queryProjects := `
	CREATE TABLE IF NOT EXISTS projects (
		id SERIAL PRIMARY KEY,
		user_id INT REFERENCES users(id) ON DELETE CASCADE,
		name VARCHAR(255) NOT NULL,
		start_url TEXT NOT NULL,
		is_ssr BOOLEAN DEFAULT TRUE,
		crawl_started_at TIMESTAMP WITH TIME ZONE,
		crawl_finished_at TIMESTAMP WITH TIME ZONE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT unique_user_project_name_url UNIQUE(user_id, name, start_url)
	);`

	queryLinks := `
	CREATE TABLE IF NOT EXISTS links (
		id SERIAL PRIMARY KEY,
		project_id INT REFERENCES projects(id) ON DELETE CASCADE,
		url TEXT NOT NULL,
		anchor_text TEXT,
		is_nofollow BOOLEAN DEFAULT FALSE,
		custom_status INT NOT NULL DEFAULT 0,
		status_code INT NOT NULL,
		link_type VARCHAR(50) NOT NULL,
		depth_level INT NOT NULL DEFAULT 1,
		file_type VARCHAR(50),
		parent_url TEXT,
		redirect_chain JSONB
	);`

	queryIndexes := `
	CREATE INDEX IF NOT EXISTS idx_links_project_id ON links(project_id);
	CREATE INDEX IF NOT EXISTS idx_links_project_status ON links(project_id, status_code);
	CREATE INDEX IF NOT EXISTS idx_links_project_custom ON links(project_id, custom_status);
	CREATE INDEX IF NOT EXISTS idx_links_project_type ON links(project_id, link_type);
	CREATE INDEX IF NOT EXISTS idx_links_project_depth ON links(project_id, depth_level);
	CREATE INDEX IF NOT EXISTS idx_links_url ON links(url);
	`

	if _, err := p.Pool.Exec(ctx, queryUsers); err != nil {
		return err
	}

	if _, err := p.Pool.Exec(ctx, queryProjects); err != nil {
		return err
	}

	if _, err := p.Pool.Exec(ctx, queryLinks); err != nil {
		return err
	}

	if _, err := p.Pool.Exec(ctx, queryIndexes); err != nil {
		return err
	}

	return nil
}

func (p *PostgresStorage) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, email, password, created_at FROM users WHERE email = $1`

	var u domain.User
	err := p.Pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.Password, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (p *PostgresStorage) CreateUser(ctx context.Context, email, hashedPassword string) (int, error) {
	query := `INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id`

	var id int
	err := p.Pool.QueryRow(ctx, query, email, hashedPassword).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, domain.ErrUserAlreadyExists
		}
		return 0, fmt.Errorf("failed to create user: %w", err)
	}
	return id, nil
}

func (p *PostgresStorage) CreateProject(ctx context.Context, userID int, name, startURL string, startedAt time.Time) (int, error) {
	var projectID int
	query := `
	INSERT INTO projects (user_id, name, start_url, crawl_started_at)
	VALUES ($1, $2, $3, $4)
	RETURNING id`

	err := p.Pool.QueryRow(ctx, query, userID, name, startURL, startedAt).Scan(&projectID)
	if err != nil {
		return 0, fmt.Errorf("failed to create project: %w", err)
	}
	return projectID, nil
}

func (p *PostgresStorage) SaveProjectLinksAndFinish(ctx context.Context, projectID int, isSSR bool, finishedAt time.Time, internal []domain.LinkResult, external []domain.LinkResult) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	updateQuery := `UPDATE projects SET is_ssr = $1, crawl_finished_at = $2 WHERE id = $3`
	if _, err := tx.Exec(ctx, updateQuery, isSSR, finishedAt, projectID); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	totalLinks := len(internal) + len(external)
	rows := make([][]any, 0, totalLinks)

	for _, link := range internal {
		chainJSON, _ := json.Marshal(link.RedirectChain)
		rows = append(rows, []any{
			projectID,
			link.URL,
			link.AnchorText,
			link.IsNoFollow,
			int(link.CustomStatus),
			link.HTTPStatus,
			"internal",
			link.DepthLevel,
			link.FileType,
			link.ParentURL,
			chainJSON,
		})
	}
	for _, link := range external {
		chainJSON, _ := json.Marshal(link.RedirectChain)
		rows = append(rows, []any{
			projectID,
			link.URL,
			link.AnchorText,
			link.IsNoFollow,
			int(link.CustomStatus),
			link.HTTPStatus,
			"external",
			link.DepthLevel,
			link.FileType,
			link.ParentURL,
			chainJSON,
		})
	}

	columns := []string{
		"project_id",
		"url",
		"anchor_text",
		"is_nofollow",
		"custom_status",
		"status_code",
		"link_type",
		"depth_level",
		"file_type",
		"parent_url",
		"redirect_chain",
	}

	_, err = tx.CopyFrom(
		ctx,
		pgx.Identifier{"links"},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to bulk copy links: %w", err)
	}

	return tx.Commit(ctx)
}

func (p *PostgresStorage) GetUserProjects(ctx context.Context, userID int) ([]domain.Project, error) {
	query := `SELECT id, user_id, name, start_url, is_ssr, crawl_started_at, crawl_finished_at, created_at
			FROM projects WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := p.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		var proj domain.Project

		if err := rows.Scan(
			&proj.ID,
			&proj.UserID,
			&proj.Name,
			&proj.StartURL,
			&proj.IsSSR,
			&proj.CrawlStartedAt,
			&proj.CrawlFinishedAt,
			&proj.CreatedAt,
		); err != nil {
			return nil, err
		}
		if proj.CrawlFinishedAt == nil {
			proj.Status = "crawling"
		} else {
			proj.Status = "completed"
		}
		projects = append(projects, proj)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return projects, nil
}

func (p *PostgresStorage) GetProjectDetails(ctx context.Context, projectID int) ([]domain.LinkResult, []domain.LinkResult, error) {
	query := `SELECT url,COALESCE(anchor_text, ''), is_nofollow, custom_status, status_code,
				depth_level, COALESCE(file_type, ''), link_type, COALESCE(parent_url, ''), redirect_chain
			 FROM links WHERE project_id = $1`

	rows, err := p.Pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var internal []domain.LinkResult
	var external []domain.LinkResult

	for rows.Next() {
		var link domain.LinkResult
		var linkType string
		var chainJSON []byte

		if err := rows.Scan(&link.URL, &link.AnchorText, &link.IsNoFollow, &link.CustomStatus, &link.HTTPStatus, &link.DepthLevel, &link.FileType, &linkType, &link.ParentURL, &chainJSON); err != nil {
			return nil, nil, err
		}

		if len(chainJSON) > 0 {
			_ = json.Unmarshal(chainJSON, &link.RedirectChain)
		}

		if linkType == "internal" {
			internal = append(internal, link)
		} else {
			external = append(external, link)
		}
	}

	return internal, external, nil
}

func (p *PostgresStorage) GetProjectHierarchyDetails(ctx context.Context, projectID int) ([]domain.LinkHierarchyNode, error) {
	query := `SELECT url, COALESCE(anchor_text, ''), is_nofollow, custom_status, status_code, COALESCE(parent_url, ''), redirect_chain
					  FROM links WHERE project_id = $1 ORDER BY parent_url`

	rows, err := p.Pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]domain.LinkResult)
	var seenURLs = make(map[string]bool)

	for rows.Next() {
		var link domain.LinkResult
		var chainJSON []byte

		if err := rows.Scan(&link.URL, &link.AnchorText, &link.IsNoFollow, &link.CustomStatus, &link.HTTPStatus, &link.ParentURL, &chainJSON); err != nil {
			return nil, err
		}

		if len(chainJSON) > 0 {
			_ = json.Unmarshal(chainJSON, &link.RedirectChain)
		}

		uniqueKey := link.ParentURL + "->" + link.URL
		if seenURLs[uniqueKey] {
			link.CustomStatus = domain.StatusDuplicateURL
		} else {
			seenURLs[uniqueKey] = true
		}
		parent := link.ParentURL
		if parent == "" {
			parent = "Root / Start Page"
		}

		grouped[parent] = append(grouped[parent], link)
	}
	var hierarchy []domain.LinkHierarchyNode
	for parent, links := range grouped {
		hierarchy = append(hierarchy, domain.LinkHierarchyNode{
			ParentURL: parent,
			Links:     links,
		})
	}

	return hierarchy, nil
}

func (p *PostgresStorage) SearchProjectLinks(ctx context.Context, projectID int, f domain.LinkSearchFilter) ([]domain.LinkResult, domain.SearchResultMeta, error) {
	baseQuery := `
			SELECT url, COALESCE(anchor_text, ''), is_nofollow, custom_status, status_code,
				link_type, depth_level, COALESCE(file_type, ''), COALESCE(parent_url, ''), redirect_chain,
				COUNT(*) OVER() AS total_count,
				COUNT(*) FILTER (WHERE link_type = 'internal') OVER() AS internal_count,
				COUNT(*) FILTER (WHERE link_type = 'external') OVER() AS external_count
			FROM links
			WHERE project_id = $1`

	args := []interface{}{projectID}

	if len(f.CustomStatuses) > 0 {
		clause := buildInClause(len(args)+1, len(f.CustomStatuses))
		baseQuery += fmt.Sprintf(" AND custom_status IN (%s)", clause)
		for _, v := range f.CustomStatuses {
			args = append(args, v)
		}
	}

	if len(f.HTTPStatuses) > 0 {
		clause := buildInClause(len(args)+1, len(f.HTTPStatuses))
		baseQuery += fmt.Sprintf(" AND status_code IN (%s)", clause)
		for _, v := range f.HTTPStatuses {
			args = append(args, v)
		}
	}

	if f.HTTPStatusMin != nil {
		args = append(args, *f.HTTPStatusMin)
		baseQuery += fmt.Sprintf(" AND status_code >= $%d", len(args))
	}

	if f.HTTPStatusMax != nil {
		args = append(args, *f.HTTPStatusMax)
		baseQuery += fmt.Sprintf(" AND status_code <= $%d", len(args))
	}

	if f.LinkType != "" {
		args = append(args, f.LinkType)
		baseQuery += fmt.Sprintf(" AND link_type = $%d", len(args))
	}

	if len(f.DepthLevels) > 0 {
		clause := buildInClause(len(args)+1, len(f.DepthLevels))
		baseQuery += fmt.Sprintf(" AND depth_level IN (%s)", clause)
		for _, v := range f.DepthLevels {
			args = append(args, v)
		}
	}

	if f.IsNoFollow != nil {
		args = append(args, *f.IsNoFollow)
		baseQuery += fmt.Sprintf(" AND is_nofollow = $%d", len(args))
	}

	if f.ParentURL != "" {
		args = append(args, "%"+f.ParentURL+"%")
		baseQuery += fmt.Sprintf(" AND parent_url ILIKE $%d", len(args))
	}

	if f.URLContains != "" {
		args = append(args, "%"+f.URLContains+"%")
		baseQuery += fmt.Sprintf(" AND url ILIKE $%d", len(args))
	}

	if f.URLStartsWith != "" {
		args = append(args, f.URLStartsWith+"%")
		baseQuery += fmt.Sprintf(" AND url ILIKE $%d", len(args))
	}

	if f.URLEndsWith != "" {
		args = append(args, "%"+f.URLEndsWith)
		baseQuery += fmt.Sprintf(" AND url ILIKE $%d", len(args))
	}

	if f.AnchorContains != "" {
		args = append(args, "%"+f.AnchorContains+"%")
		baseQuery += fmt.Sprintf(" AND anchor_text ILIKE $%d", len(args))
	}

	baseQuery += " ORDER BY depth_level ASC, id ASC"

	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := p.Pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, domain.SearchResultMeta{}, err
	}
	defer rows.Close()

	var results []domain.LinkResult
	var meta domain.SearchResultMeta
	meta.Limit = limit
	meta.Offset = offset

	for rows.Next() {
		var link domain.LinkResult
		var linkType string
		var chainJSON []byte
		var totalCount, internalCount, externalCount int

		err := rows.Scan(
			&link.URL,
			&link.AnchorText,
			&link.IsNoFollow,
			&link.CustomStatus,
			&link.HTTPStatus,
			&linkType,
			&link.DepthLevel,
			&link.FileType,
			&link.ParentURL,
			&chainJSON,
			&totalCount,
			&internalCount,
			&externalCount,
		)
		if err != nil {
			return nil, domain.SearchResultMeta{}, err
		}

		if len(chainJSON) > 0 {
			_ = json.Unmarshal(chainJSON, &link.RedirectChain)
		}

		meta.TotalLinks = totalCount
		meta.InternalLinks = internalCount
		meta.ExternalLinks = externalCount

		results = append(results, link)
	}

	return results, meta, nil
}

func (p *PostgresStorage) ProjectExists(ctx context.Context, userID int, name, startURL string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM projects WHERE user_id = $1 AND name = $2 AND start_url = $3)`

	var exists bool
	err := p.Pool.QueryRow(ctx, query, userID, name, startURL).Scan(&exists)
	return exists, err
}

func (p *PostgresStorage) ProjectBelongsToUser(ctx context.Context, projectID, userID int) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1 AND user_id = $2)`
	var exists bool
	err := p.Pool.QueryRow(ctx, query, projectID, userID).Scan(&exists)
	return exists, err
}

func buildInClause(startIdx int, count int) string {
	placeholders := make([]string, count)
	for i := 0; i < count; i++ {
		placeholders[i] = fmt.Sprintf("$%d", startIdx+i)
	}
	return strings.Join(placeholders, ", ")
}
