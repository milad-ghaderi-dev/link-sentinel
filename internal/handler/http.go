package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/auth"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/domain"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/service"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type CrawlRequest struct {
	Name     string `json:"name" binding:"required"`
	URL      string `json:"url" binding:"required,url"`
	MaxPages int    `json:"max_pages"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type Server struct {
	authService  *service.AuthService
	crawlService *service.CrawlerService
	tokenManager *auth.TokenManager
}

func NewServer(authService *service.AuthService, crawlerService *service.CrawlerService, tokenManager *auth.TokenManager) *Server {
	return &Server{
		authService:  authService,
		crawlService: crawlerService,
		tokenManager: tokenManager,
	}
}

func (s *Server) RegisterRoutes(r *gin.Engine) {
	r.POST("/api/auth/login", s.handleLogin)
	r.POST("/api/auth/register", s.handleRegister)
	protected := r.Group("/api")
	protected.Use(AuthMiddleware(s.tokenManager))
	{
		protected.POST("/crawl", s.handleCrawl)
		protected.GET("/crawl/status/:job_id", s.handleCrawlStatus)
		protected.GET("/projects", s.handleGetProjects)
		protected.GET("/projects/:id", s.handleGetProjectDetails)
		protected.GET("/projects/:id/tree", s.handleGetProjectHierarchy)
		protected.POST("/projects/:id/search", s.handleSearchProjectLinks)
	}
}

func AuthMiddleware(tokenManager *auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Authorization header required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid Authorization header format"})
			c.Abort()
			return
		}

		claims, err := tokenManager.ValidateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": "Invalid or expired token"})
			c.Abort()
			return
		}

		c.Set("userID", claims.UserID)
		c.Next()
	}
}

func (s *Server) handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid email or password input"})
		return
	}

	token, err := s.authService.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Login failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"token":  token,
	})
}

func (s *Server) handleRegister(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid email or password (min 6 characters)"})
		return
	}

	token, err := s.authService.Register(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, domain.ErrUserAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Registration failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "success",
		"message": "User registered successfully",
		"token":   token,
	})
}

func (s *Server) handleCrawl(c *gin.Context) {
	userID := c.GetInt("userID")

	var req CrawlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid or missing URL"})
		return
	}

	projectID, jobID, err := s.crawlService.StartCrawl(c.Request.Context(), userID, req.Name, req.URL, req.MaxPages)
	if err != nil {
		if errors.Is(err, domain.ErrProjectAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to start crawl job"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "processing",
		"message":    "Crawl job registered and running",
		"project_id": projectID,
		"job_id":     jobID,
	})
}

func (s *Server) handleCrawlStatus(c *gin.Context) {
	jobID := c.Param("job_id")

	progress, err := s.crawlService.GetJobProgress(jobID)
	if err != nil {
		if errors.Is(err, domain.ErrJobNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"job_id": jobID,
		"data":   progress,
	})
}

func (s *Server) handleGetProjects(c *gin.Context) {
	userID := c.GetInt("userID")
	projects, err := s.crawlService.GetUserProjects(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch projects"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   projects,
	})
}

func (s *Server) handleGetProjectDetails(c *gin.Context) {
	userID := c.GetInt("userID")
	idParam := c.Param("id")
	projectID, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid project ID"})
		return
	}

	internal, external, err := s.crawlService.GetProjectDetails(c.Request.Context(), projectID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to get project details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data": gin.H{
			"internal": internal,
			"external": external,
		},
	})
}

func (s *Server) handleGetProjectHierarchy(c *gin.Context) {
	userID := c.GetInt("userID")
	idParam := c.Param("id")
	projectID, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid project ID"})
		return
	}

	hierarchy, err := s.crawlService.GetProjectHierarchy(c.Request.Context(), projectID, userID)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to fetch project hierarchy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   hierarchy,
	})
}

func (s *Server) handleSearchProjectLinks(c *gin.Context) {
	userID := c.GetInt("userID")
	idParam := c.Param("id")
	projectID, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid project ID"})
		return
	}

	var filter domain.LinkSearchFilter
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&filter); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid JSON format: " + err.Error()})
			return
		}
	}

	results, meta, err := s.crawlService.SearchProjectLinks(c.Request.Context(), projectID, userID, filter)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Search failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"meta":   meta,
		"data":   results,
	})
}
