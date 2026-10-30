package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/milad-ghaderi-dev/link-sentinel/internal/crawler"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/repository"
)

type CrawlerRepository interface {
	ProjectExists(ctx context.Context, userID int, name, startURL string) (bool, error)
	CreateProject(ctx context.Context, userID int, name, startURL string, startTime time.Time) (int, error)
	SaveProjectLinksAndFinish(ctx context.Context, projectID int, isSSR bool, finishedAt time.Time, internal []domain.LinkResult, external []domain.LinkResult) error
	GetUserProjects(ctx context.Context, userID int) ([]domain.Project, error)
	GetProjectDetails(ctx context.Context, projectID int) ([]domain.LinkResult, []domain.LinkResult, error)
	GetProjectHierarchyDetails(ctx context.Context, projectID int) ([]domain.LinkHierarchyNode, error)
	SearchProjectLinks(ctx context.Context, projectID int, filter domain.LinkSearchFilter) ([]domain.LinkResult, domain.SearchResultMeta, error)
	ProjectBelongsToUser(ctx context.Context, projectID, userID int) (bool, error)
}

type CrawlerService struct {
	repo         CrawlerRepository
	crawlManager *crawler.SingleFlightManager
	appCtx       context.Context
	activeJobs   map[string]*repository.SafeMemoryStorage
	cachedProg   map[string]domain.CrawlProgress
	activeJobsMu sync.RWMutex
}

func NewCrawlerService(repo CrawlerRepository, appCtx ...context.Context) *CrawlerService {
	ctx := context.Background()
	if len(appCtx) > 0 && appCtx[0] != nil {
		ctx = appCtx[0]
	}
	return &CrawlerService{
		repo:         repo,
		crawlManager: crawler.NewSingleFlightManager(),
		appCtx:       ctx,
		activeJobs:   make(map[string]*repository.SafeMemoryStorage),
		cachedProg:   make(map[string]domain.CrawlProgress),
	}
}

func (s *CrawlerService) StartCrawl(ctx context.Context, userID int, name, targetURL string, requestedMaxPages int) (int, string, error) {
	exists, err := s.repo.ProjectExists(ctx, userID, name, targetURL)
	if err != nil {
		return 0, "", err
	}
	if exists {
		return 0, "", domain.ErrProjectAlreadyExists
	}

	startTime := time.Now()
	projectID, err := s.repo.CreateProject(ctx, userID, name, targetURL, startTime)
	if err != nil {
		return 0, "", err
	}

	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())
	planMaxPagesLimit := 1000
	effectiveMaxPages := 100
	if requestedMaxPages > 0 {
		if requestedMaxPages > planMaxPagesLimit {
			effectiveMaxPages = planMaxPagesLimit
		} else {
			effectiveMaxPages = requestedMaxPages
		}
	}

	memStore, executeCrawl := s.crawlManager.ExecuteOrJoin(targetURL, 10, effectiveMaxPages)

	s.activeJobsMu.Lock()
	s.activeJobs[jobID] = memStore
	s.activeJobsMu.Unlock()

	go func() {
		log.Printf("[Crawler Service] Started background crawl for project %d (job %s)", projectID, jobID)
		result, err := executeCrawl(s.appCtx)
		if err != nil {
			log.Printf("[Crawler Service] Crawl job %s failed: %v", jobID, err)
		} else {
			log.Printf("[Crawler Service] Saving project %d to DB (Internal=%d, External=%d)...", projectID, len(result.Internal), len(result.External))
			saveErr := s.repo.SaveProjectLinksAndFinish(
				s.appCtx,
				projectID,
				result.IsSSR,
				result.FinishedAt,
				result.Internal,
				result.External,
			)
			if saveErr != nil {
				log.Printf("[Crawler Service] ERROR saving project %d to DB: %v", projectID, saveErr)
			} else {
				log.Printf("[Crawler Service] Project %d successfully saved to DB", projectID)
			}
		}

		s.activeJobsMu.Lock()
		if store, exists := s.activeJobs[jobID]; exists {
			s.cachedProg[jobID] = store.GetProgress()
			delete(s.activeJobs, jobID)
		}
		s.activeJobsMu.Unlock()

		time.AfterFunc(10*time.Minute, func() {
			s.activeJobsMu.Lock()
			delete(s.cachedProg, jobID)
			s.activeJobsMu.Unlock()
		})
	}()

	return projectID, jobID, nil
}

func (s *CrawlerService) GetJobProgress(jobID string) (*domain.CrawlProgress, error) {
	s.activeJobsMu.RLock()
	defer s.activeJobsMu.RUnlock()

	if memStore, exists := s.activeJobs[jobID]; exists {
		progress := memStore.GetProgress()
		return &progress, nil
	}

	if prog, exists := s.cachedProg[jobID]; exists {
		return &prog, nil
	}

	return nil, domain.ErrJobNotFound
}

func (s *CrawlerService) GetUserProjects(ctx context.Context, userID int) ([]domain.Project, error) {
	return s.repo.GetUserProjects(ctx, userID)
}

func (s *CrawlerService) GetProjectDetails(ctx context.Context, projectID, userID int) ([]domain.LinkResult, []domain.LinkResult, error) {
	belongs, err := s.repo.ProjectBelongsToUser(ctx, projectID, userID)
	if err != nil {
		return nil, nil, err
	}
	if !belongs {
		return nil, nil, domain.ErrProjectNotFound
	}
	return s.repo.GetProjectDetails(ctx, projectID)
}

func (s *CrawlerService) GetProjectHierarchy(ctx context.Context, projectID, userID int) ([]domain.LinkHierarchyNode, error) {
	belongs, err := s.repo.ProjectBelongsToUser(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, domain.ErrProjectNotFound
	}
	return s.repo.GetProjectHierarchyDetails(ctx, projectID)
}

func (s *CrawlerService) SearchProjectLinks(ctx context.Context, projectID, userID int, filter domain.LinkSearchFilter) ([]domain.LinkResult, domain.SearchResultMeta, error) {
	belongs, err := s.repo.ProjectBelongsToUser(ctx, projectID, userID)
	if err != nil {
		return nil, domain.SearchResultMeta{}, err
	}
	if !belongs {
		return nil, domain.SearchResultMeta{}, domain.ErrProjectNotFound
	}
	return s.repo.SearchProjectLinks(ctx, projectID, filter)
}
