package routes

import (
	"net/http"

	"github.com/dacbd/llm-proxy/internal/config"
	"github.com/dacbd/llm-proxy/internal/handler"
	"github.com/dacbd/llm-proxy/internal/handler/ollama"
	"github.com/dacbd/llm-proxy/weave"
)

type RouteRegister func(mux *http.ServeMux) error

func SetupRoutes(mux *http.ServeMux, routes ...RouteRegister) error {
	for _, rr := range routes {
		if err := rr(mux); err != nil {
			return err
		}
	}
	return nil
}

func RegisterK8sRoutes(mux *http.ServeMux) error {
	k8s := handler.NewK8sHandler()
	mux.HandleFunc("GET /health", k8s.Health)
	mux.HandleFunc("GET /ready", k8s.Ready)
	return nil
}

func RegisterOllamaRoutes(mux *http.ServeMux, cfg *config.ServerConfig, weaveClient weave.API) error {
	h := ollama.NewHandler(cfg.OllamaURL, weaveClient)
	mux.HandleFunc("POST /api/generate", h.Generate)
	mux.HandleFunc("POST /api/chat", h.Chat)
	mux.HandleFunc("GET /api/tags", h.List)
	mux.HandleFunc("GET /api/ps", h.ListRunning)
	return nil
}

func RegisterRoutes(mux *http.ServeMux, cfg *config.ServerConfig, weaveClient weave.API) error {
	return SetupRoutes(mux,
		RegisterK8sRoutes,
		func(mux *http.ServeMux) error { return RegisterOllamaRoutes(mux, cfg, weaveClient) },
	)
}
