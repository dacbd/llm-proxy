package routes

import (
	"net/http"

	"github.com/dacbd/llm-proxy/internal/config"
	"github.com/dacbd/llm-proxy/internal/handler"
	ollamahandler "github.com/dacbd/llm-proxy/internal/handler/ollama"
	"github.com/dacbd/llm-proxy/internal/handler/openai"
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
	h := ollamahandler.NewHandler(cfg.GetUpstreamURL(), weaveClient)
	mux.HandleFunc("POST /api/generate", h.Generate)
	mux.HandleFunc("POST /api/chat", h.Chat)
	mux.HandleFunc("GET /api/tags", h.List)
	mux.HandleFunc("GET /api/ps", h.ListRunning)
	return nil
}

func RegisterOpenAIRoutes(mux *http.ServeMux, cfg *config.ServerConfig, weaveClient weave.API) error {
	h := openai.NewHandler(cfg.GetUpstreamURL(), weaveClient)
	mux.HandleFunc("POST /v1/chat/completions", h.ChatCompletions)
	mux.HandleFunc("POST /v1/completions", h.Completions)
	mux.HandleFunc("POST /v1/embeddings", h.Embeddings)
	mux.HandleFunc("GET /v1/models", h.ListModels)
	mux.HandleFunc("POST /v1/images/generations", h.CreateImage)
	return nil
}

func RegisterRoutes(mux *http.ServeMux, cfg *config.ServerConfig, weaveClient weave.API) error {
	routes := []RouteRegister{
		RegisterK8sRoutes,
		func(mux *http.ServeMux) error { return RegisterOllamaRoutes(mux, cfg, weaveClient) },
	}

	// Register OpenAI-compatible routes when upstream type is "openai"
	// This coexists with Ollama routes
	if cfg.GetUpstreamType() == "openai" {
		routes = append(routes, func(mux *http.ServeMux) error {
			return RegisterOpenAIRoutes(mux, cfg, weaveClient)
		})
	}

	return SetupRoutes(mux, routes...)
}
