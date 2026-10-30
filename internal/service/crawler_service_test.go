package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
)

type mockCrawlerRepo struct {
	projectExists    bool
	projectExistsErr error
	createdProjectID int
	createProjectErr error
	saveLinksErr     error
	userProjects     []domain.Project
	userProjectsErr  error
	detailsInternal  []domain.LinkResult
	detailsExternal  []domain.LinkResult
	detailsErr       error
	hierarchy        []domain.LinkHierarchyNode
	hierarchyErr     error
	searchResults    []domain.LinkResult
	searchMeta       domain.SearchResultMeta
	searchErr        error
	belongsToUser    bool
	belongsToUserErr error
}

func (m *mockCrawlerRepo) ProjectExists(ctx context.Context, userID int, name, startURL string) (bool, error) {
	return m.projectExists, m.projectExistsErr
}

func (m *mockCrawlerRepo) CreateProject(ctx context.Context, userID int, name, startURL string, startTime time.Time) (int, error) {
	return m.createdProjectID, m.createProjectErr
}

func (m *mockCrawlerRepo) SaveProjectLinksAndFinish(ctx context.Context, projectID int, isSSR bool, finishedAt time.Time, internal []domain.LinkResult, external []domain.LinkResult) error {
	return m.saveLinksErr
}

func (m *mockCrawlerRepo) GetUserProjects(ctx context.Context, userID int) ([]domain.Project, error) {
	return m.userProjects, m.userProjectsErr
}

func (m *mockCrawlerRepo) GetProjectDetails(ctx context.Context, projectID int) ([]domain.LinkResult, []domain.LinkResult, error) {
	return m.detailsInternal, m.detailsExternal, m.detailsErr
}

func (m *mockCrawlerRepo) GetProjectHierarchyDetails(ctx context.Context, projectID int) ([]domain.LinkHierarchyNode, error) {
	return m.hierarchy, m.hierarchyErr
}

func (m *mockCrawlerRepo) SearchProjectLinks(ctx context.Context, projectID int, filter domain.LinkSearchFilter) ([]domain.LinkResult, domain.SearchResultMeta, error) {
	return m.searchResults, m.searchMeta, m.searchErr
}

func (m *mockCrawlerRepo) ProjectBelongsToUser(ctx context.Context, projectID, userID int) (bool, error) {
	return m.belongsToUser, m.belongsToUserErr
}

func TestCrawlerService_StartCrawl_ProjectAlreadyExists(t *testing.T) {
	mockRepo := &mockCrawlerRepo{
		projectExists: true,
	}

	svc := NewCrawlerService(mockRepo)
	_, _, err := svc.StartCrawl(context.Background(), 1, "Duplicate Test", "https://example.com", 0)
	if !errors.Is(err, domain.ErrProjectAlreadyExists) {
		t.Fatalf("expected ErrProjectAlreadyExists, got: %v", err)
	}
}

func TestCrawlerService_GetProjectDetails_IDOR_Protection(t *testing.T) {
	mockRepo := &mockCrawlerRepo{
		belongsToUser: false,
	}

	svc := NewCrawlerService(mockRepo)
	_, _, err := svc.GetProjectDetails(context.Background(), 100, 2)
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound for unauthorized project access, got: %v", err)
	}
}

func TestCrawlerService_GetProjectDetails_Success(t *testing.T) {
	mockRepo := &mockCrawlerRepo{
		belongsToUser: true,
		detailsInternal: []domain.LinkResult{
			{URL: "https://example.com/about", HTTPStatus: 200},
		},
		detailsExternal: []domain.LinkResult{
			{URL: "https://github.com", HTTPStatus: 200},
		},
	}

	svc := NewCrawlerService(mockRepo)
	internal, external, err := svc.GetProjectDetails(context.Background(), 100, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(internal) != 1 || len(external) != 1 {
		t.Fatalf("expected 1 internal and 1 external link, got %d and %d", len(internal), len(external))
	}
}

func TestCrawlerService_GetUserProjects_Success(t *testing.T) {
	mockRepo := &mockCrawlerRepo{
		userProjects: []domain.Project{
			{ID: 1, Name: "Project A"},
			{ID: 2, Name: "Project B"},
		},
	}

	svc := NewCrawlerService(mockRepo)
	projects, err := svc.GetUserProjects(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}

func TestCrawlerService_GetJobProgress_JobNotFound(t *testing.T) {
	mockRepo := &mockCrawlerRepo{}
	svc := NewCrawlerService(mockRepo)

	_, err := svc.GetJobProgress("non_existent_job_id")
	if !errors.Is(err, domain.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got: %v", err)
	}
}

func TestCrawlerService_GetJobProgress_FromCachedProgress(t *testing.T) {
	mockRepo := &mockCrawlerRepo{}
	svc := NewCrawlerService(mockRepo)

	svc.activeJobsMu.Lock()
	svc.cachedProg["job_finished_123"] = domain.CrawlProgress{
		TotalFound:    50,
		InternalFound: 40,
		ExternalFound: 10,
		IsFinished:    true,
	}
	svc.activeJobsMu.Unlock()

	prog, err := svc.GetJobProgress("job_finished_123")
	if err != nil {
		t.Fatalf("unexpected error fetching cached progress: %v", err)
	}

	if !prog.IsFinished || prog.TotalFound != 50 {
		t.Fatalf("expected finished with 50 total links, got %+v", prog)
	}
}

func TestCrawlerService_SearchProjectLinks_IDOR(t *testing.T) {
	mockRepo := &mockCrawlerRepo{belongsToUser: false}
	svc := NewCrawlerService(mockRepo)

	_, _, err := svc.SearchProjectLinks(context.Background(), 10, 2, domain.LinkSearchFilter{})
	if !errors.Is(err, domain.ErrProjectNotFound) {
		t.Fatalf("expected ErrProjectNotFound, got: %v", err)
	}
}
