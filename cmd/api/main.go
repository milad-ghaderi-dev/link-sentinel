package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/auth"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/config"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/handler"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/repository"
	"github.com/milad-ghaderi-dev/link-sentinel/internal/service"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()

	db, err := repository.NewPostgresStorage(appCtx, cfg.DB.DSN())
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Pool.Close()

	tokenManager := auth.NewTokenManager(cfg.JWTSecret)
	authService := service.NewAuthService(db, tokenManager)
	crawlerService := service.NewCrawlerService(db, appCtx)
	r := gin.Default()
	s := handler.NewServer(authService, crawlerService, tokenManager)
	s.RegisterRoutes(r)

	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("LinkSentinel API server starting on port %s...", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server listen failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down LinkSentinel server gracefully...")
	cancelApp()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}
	log.Println("LinkSentinel server exited cleanly.")
}
