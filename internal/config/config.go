package config

import (
	"embed"
	"flag"
)

//go:embed assets/fonts/JetBrainsMono-Regular.ttf
var Fonts embed.FS

var (
	BindIP   = flag.String("ip", "127.0.0.1", "server bind IP address")
	BindPORT = flag.Int("port", 8080, "server port")
	Display  = flag.String("display", ":0", "X11 display to use (e.g., :0, :1, :2)")
	DPI      = flag.Float64("dpi", 72, "screen resolution in Dots Per Inch")
	Fontfile = flag.String("fontfile", "assets/fonts/JetBrainsMono-Regular.ttf", "filename of the ttf font")
	Hinting  = flag.String("hinting", "none", "none | full")
	Size     = flag.Float64("size", 9, "font size in points")
	Spacing  = flag.Float64("spacing", 1.5, "line spacing (e.g. 2 means double spaced)")
	Wonb     = flag.Bool("whiteonblack", false, "white text on a black background")

	// LLM Configuration
	Provider = flag.String("provider", "deepseek", "LLM provider to use (deepseek, zai)")
	APIKey   = flag.String("key", "", "LLM API key")
	Model    = flag.String("model", "", "LLM model name")
	BaseURL  = flag.String("base-url", "", "LLM base URL (optional, uses provider default if not specified)")
)

// LLMConfig holds the LLM configuration
type LLMConfig struct {
	Provider string
	APIKey   string
	Model    string
	BaseURL  string
}

// GetLLMConfig returns the current LLM configuration
func GetLLMConfig() LLMConfig {
	return LLMConfig{
		Provider: *Provider,
		APIKey:   *APIKey,
		Model:    *Model,
		BaseURL:  *BaseURL,
	}
}
