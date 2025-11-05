package config

import (
	"flag"
	"os"
)

type Config struct {
	BindIP   *string
	BindPORT *int
	Display  *string
	Provider *string
	BaseURL  *string
	APIKey   *string
	Model    *string
}

func New() *Config {
	cfg := &Config{
		BindIP:   flag.String("ip", "0.0.0.0", "IP address to bind to"),
		BindPORT: flag.Int("port", 8081, "Port to bind to"),
		Display:  flag.String("display", ":0", "X11 display to use"),
		Provider: flag.String("provider", "", "LLM provider (deepseek, zai)"),
		BaseURL:  flag.String("base-url", "", "LLM API base URL"),
		APIKey:   flag.String("key", "", "LLM API key"),
		Model:    flag.String("model", "", "LLM model"),
	}

	flag.Parse()

	if *cfg.Provider == "" {
		*cfg.Provider = os.Getenv("LLM_PROVIDER")
	}
	if *cfg.BaseURL == "" {
		*cfg.BaseURL = os.Getenv("LLM_BASE_URL")
	}
	if *cfg.APIKey == "" {
		*cfg.APIKey = os.Getenv("LLM_API_KEY")
	}
	if *cfg.Model == "" {
		*cfg.Model = os.Getenv("LLM_MODEL")
	}

	return cfg
}

func GetLLMConfig() LLMConfig {
	provider := os.Getenv("LLM_PROVIDER")
	baseURL := os.Getenv("LLM_BASE_URL")
	apiKey := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL")

	return LLMConfig{
		Provider: provider,
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    model,
	}
}

type LLMConfig struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}
