package config

import (
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port         int    `mapstructure:"port"`
	OllamaURL    string `mapstructure:"ollama-url"`
	WandbAPIKey  string `mapstructure:"wandb-api-key"`
	WandbProject string `mapstructure:"wandb-project"`
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
