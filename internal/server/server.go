package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dacbd/llm-proxy/internal/config"
	"github.com/dacbd/llm-proxy/internal/middleware"
	"github.com/dacbd/llm-proxy/internal/routes"
	"github.com/dacbd/llm-proxy/weave"
)

type Server struct {
	httpServer *http.Server
	config     *config.ServerConfig
}

func NewServer(cfg *config.ServerConfig) *Server {
	var weaveClient weave.API
	if cfg.WeaveEnabled() {
		weaveClient = weave.NewClient(cfg.WandbAPIKey, cfg.WandbProject)
		slog.Info("Weave tracing enabled", "project", cfg.WandbProject)
	}

	mux := http.NewServeMux()
	if err := routes.RegisterRoutes(mux, cfg, weaveClient); err != nil {
		slog.Error("Failed to register routes", "error", err)
		os.Exit(1)
	}

	middlewares := middleware.CreateStack(
		middleware.Logging,
	)

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.Port),
			Handler:      middlewares(mux),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 300 * time.Second, // 5 minute timeout for streaming LLM responses
			IdleTimeout:  60 * time.Second,
		},
		config: cfg,
	}
}

func (s *Server) Start() error {
	slog.Info("Starting llm-proxy", "port", s.config.Port, "upstream", s.config.OllamaURL)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}
