package config

import (
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port         int    `mapstructure:"port"`
	OllamaURL    string `mapstructure:"ollama-url"`    // Deprecated: use UpstreamURL
	UpstreamURL  string `mapstructure:"upstream-url"`  // Upstream LLM server URL
	UpstreamType string `mapstructure:"upstream-type"` // "ollama" or "openai"
	WandbAPIKey  string `mapstructure:"wandb-api-key"`
	WandbProject string `mapstructure:"wandb-project"`
}

// GetUpstreamURL returns the upstream URL, preferring UpstreamURL over deprecated OllamaURL
func (c *ServerConfig) GetUpstreamURL() string {
	if c.UpstreamURL != "" {
		return c.UpstreamURL
	}
	return c.OllamaURL
}

// GetUpstreamType returns the upstream type, defaulting to "ollama" if not set
func (c *ServerConfig) GetUpstreamType() string {
	if c.UpstreamType != "" {
		return c.UpstreamType
	}
	return "ollama"
}

// WeaveEnabled reports whether Weave tracing is configured.
func (c *ServerConfig) WeaveEnabled() bool {
	return c.WandbAPIKey != "" && c.WandbProject != ""
}

func LoadServerConfig() (*ServerConfig, error) {
	cfg := &ServerConfig{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
