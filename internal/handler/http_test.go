package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/auth"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	user *domain.User
	err  error
}

func (m *mockUserRepo) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.user, m.err
}

func (m *mockUserRepo) CreateUser(ctx context.Context, email, hashedPassword string) (int, error) {
	return 1, m.err
}

type mockCrawlerRepo struct {
	projectExists    bool
	projectExistsErr error
	createdProjectID int
	belongsToUser    bool
	detailsInternal  []domain.LinkResult
	detailsExternal  []domain.LinkResult
	userProjects     []domain.Project
	searchResults    []domain.LinkResult
	searchMeta       domain.SearchResultMeta
	hierarchy        []domain.LinkHierarchyNode
}

func (m *mockCrawlerRepo) ProjectExists(ctx context.Context, userID int, name, startURL string) (bool, error) {
	return m.projectExists, m.projectExistsErr
}

func (m *mockCrawlerRepo) CreateProject(ctx context.Context, userID int, name, startURL string, startTime time.Time) (int, error) {
	return m.createdProjectID, nil
}

func (m *mockCrawlerRepo) SaveProjectLinksAndFinish(ctx context.Context, projectID int, isSSR bool, finishedAt time.Time, internal []domain.LinkResult, external []domain.LinkResult) error {
	return nil
}

func (m *mockCrawlerRepo) GetUserProjects(ctx context.Context, userID int) ([]domain.Project, error) {
	return m.userProjects, nil
}

func (m *mockCrawlerRepo) GetProjectDetails(ctx context.Context, projectID int) ([]domain.LinkResult, []domain.LinkResult, error) {
	return m.detailsInternal, m.detailsExternal, nil
}

func (m *mockCrawlerRepo) GetProjectHierarchyDetails(ctx context.Context, projectID int) ([]domain.LinkHierarchyNode, error) {
	return m.hierarchy, nil
}

func (m *mockCrawlerRepo) SearchProjectLinks(ctx context.Context, projectID int, filter domain.LinkSearchFilter) ([]domain.LinkResult, domain.SearchResultMeta, error) {
	return m.searchResults, m.searchMeta, nil
}

func (m *mockCrawlerRepo) ProjectBelongsToUser(ctx context.Context, projectID, userID int) (bool, error) {
	return m.belongsToUser, nil
}

func setupTestRouter(userRepo service.UserRepository, crawlerRepo service.CrawlerRepository, tm *auth.TokenManager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	authSvc := service.NewAuthService(userRepo, tm)
	crawlSvc := service.NewCrawlerService(crawlerRepo)
	server := NewServer(authSvc, crawlSvc, tm)
	server.RegisterRoutes(r)
	return r
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tm := auth.NewTokenManager("test_middleware_secret")
	validToken, _ := tm.GenerateToken(99, "tester@linky.dev")

	r := gin.New()
	r.Use(AuthMiddleware(tm))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetInt("userID")})
	})

	tests := []struct {
		name       string
		authHeader string
		wantCode   int
	}{
		{name: "missing header", authHeader: "", wantCode: http.StatusUnauthorized},
		{name: "invalid format", authHeader: "Basic " + validToken, wantCode: http.StatusUnauthorized},
		{name: "invalid token", authHeader: "Bearer bad.token.here", wantCode: http.StatusUnauthorized},
		{name: "valid token", authHeader: "Bearer " + validToken, wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/protected", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			r.ServeHTTP(w, req)
			if w.Code != tt.wantCode {
				t.Fatalf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestHandleLogin(t *testing.T) {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	mockUser := &domain.User{
		ID:       1,
		Email:    "admin@link-sentinel.dev",
		Password: string(hashedPassword),
	}
	userRepo := &mockUserRepo{user: mockUser}
	crawlerRepo := &mockCrawlerRepo{}
	tm := auth.NewTokenManager("test_secret_for_login")

	r := setupTestRouter(userRepo, crawlerRepo, tm)

	t.Run("invalid json body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBufferString("not-json"))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for bad json, got: %d", w.Code)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		body, _ := json.Marshal(LoginRequest{Email: "admin@link-sentinel.dev", Password: "wrong"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 for wrong credentials, got: %d", w.Code)
		}
	})

	t.Run("successful login", func(t *testing.T) {
		body, _ := json.Marshal(LoginRequest{Email: "admin@link-sentinel.dev", Password: "secret123"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for valid login, got: %d", w.Code)
		}

		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "success" || resp["token"] == nil {
			t.Fatalf("expected success status and token in response, got: %v", resp)
		}
	})
}

func TestHandleCrawl_ValidationAndConflict(t *testing.T) {
	tm := auth.NewTokenManager("test_secret_crawl")
	validToken, _ := tm.GenerateToken(10, "crawler@link-sentinel.dev")

	t.Run("duplicate project returns 409", func(t *testing.T) {
		crawlerRepo := &mockCrawlerRepo{projectExists: true}
		r := setupTestRouter(&mockUserRepo{}, crawlerRepo, tm)

		body, _ := json.Marshal(CrawlRequest{Name: "Duplicate", URL: "https://example.com"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/crawl", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got: %d", w.Code)
		}
	})

	t.Run("invalid URL returns 400", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{}, tm)

		body, _ := json.Marshal(CrawlRequest{Name: "Bad URL", URL: "not-a-valid-url"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/crawl", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for bad URL, got: %d", w.Code)
		}
	})
}

func TestHandleGetProjectDetails_IDOR(t *testing.T) {
	tm := auth.NewTokenManager("test_secret_idor")
	validToken, _ := tm.GenerateToken(5, "user5@link-sentinel.dev")

	t.Run("unauthorized project access returns 404", func(t *testing.T) {
		crawlerRepo := &mockCrawlerRepo{belongsToUser: false}
		r := setupTestRouter(&mockUserRepo{}, crawlerRepo, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/projects/999", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for accessing other user's project, got: %d", w.Code)
		}
	})

	t.Run("invalid project id format returns 400", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/projects/abc", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for non-numeric project id, got: %d", w.Code)
		}
	})
}

func TestHandleRegister(t *testing.T) {
	tm := auth.NewTokenManager("test_secret_reg")

	t.Run("successful registration returns 201", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{}, tm)

		body, _ := json.Marshal(RegisterRequest{Email: "fresh@link-sentinel.dev", Password: "secure_pass"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got: %d", w.Code)
		}
	})

	t.Run("duplicate email returns 409", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{err: domain.ErrUserAlreadyExists}, &mockCrawlerRepo{}, tm)

		body, _ := json.Marshal(RegisterRequest{Email: "duplicate@link-sentinel.dev", Password: "secure_pass"})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for existing user, got: %d", w.Code)
		}
	})
}

func TestHandleSearchProjectLinks(t *testing.T) {
	tm := auth.NewTokenManager("test_secret_search")
	validToken, _ := tm.GenerateToken(7, "searcher@link-sentinel.dev")

	t.Run("invalid project id format returns 400", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/projects/not-a-number/search", bytes.NewBufferString("{}"))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got: %d", w.Code)
		}
	})

	t.Run("invalid JSON filter body returns 400", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{belongsToUser: true}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/projects/12/search", bytes.NewBufferString("invalid json"))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request for malformed JSON, got: %d", w.Code)
		}
	})

	t.Run("IDOR check returns 404 when project not owned by user", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{belongsToUser: false}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/projects/99/search", bytes.NewBufferString("{}"))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for IDOR attempt, got: %d", w.Code)
		}
	})

	t.Run("successful search returns 200 with meta and results", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{
			belongsToUser: true,
			searchResults: []domain.LinkResult{
				{URL: "https://example.com/golang", HTTPStatus: 200},
			},
			searchMeta: domain.SearchResultMeta{
				TotalLinks:    1,
				InternalLinks: 1,
				ExternalLinks: 0,
				Limit:         10,
				Offset:        0,
			},
		}, tm)

		filter := domain.LinkSearchFilter{
			URLContains: "golang",
			LinkType:    "internal",
			Limit:       10,
			Offset:      0,
		}
		body, _ := json.Marshal(filter)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/projects/12/search", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+validToken)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got: %d", w.Code)
		}

		var resp map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["status"] != "success" || resp["meta"] == nil || resp["data"] == nil {
			t.Fatalf("expected success with meta and data, got: %v", resp)
		}
	})
}

func TestHandleGetProjectHierarchy(t *testing.T) {
	tm := auth.NewTokenManager("test_secret_hierarchy")
	validToken, _ := tm.GenerateToken(8, "tree@link-sentinel.dev")

	t.Run("invalid project id format returns 400", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/projects/invalid/tree", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got: %d", w.Code)
		}
	})

	t.Run("unauthorized project hierarchy returns 404", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{belongsToUser: false}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/projects/55/tree", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found, got: %d", w.Code)
		}
	})

	t.Run("successful hierarchy returns 200", func(t *testing.T) {
		r := setupTestRouter(&mockUserRepo{}, &mockCrawlerRepo{
			belongsToUser: true,
			hierarchy: []domain.LinkHierarchyNode{
				{
					ParentURL: "https://example.com",
					Links:     []domain.LinkResult{{URL: "https://example.com/about"}},
				},
			},
		}, tm)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/projects/55/tree", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got: %d", w.Code)
		}
	})
}

func TestHandleGetProjects(t *testing.T) {
	tm := auth.NewTokenManager("test_secret_projects")
	validToken, _ := tm.GenerateToken(3, "proj_user@link-sentinel.dev")

	crawlerRepo := &mockCrawlerRepo{
		userProjects: []domain.Project{
			{ID: 1, Name: "Site 1", StartURL: "https://site1.com"},
			{ID: 2, Name: "Site 2", StartURL: "https://site2.com"},
		},
	}
	r := setupTestRouter(&mockUserRepo{}, crawlerRepo, tm)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer "+validToken)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "success" || resp["data"] == nil {
		t.Fatalf("expected success with data, got: %v", resp)
	}
}
