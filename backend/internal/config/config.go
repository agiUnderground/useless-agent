package config

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed assets/fonts/JetBrainsMono-Regular.ttf
var Fonts embed.FS

var (
	BindIP   = flag.String("ip", "127.0.0.1", "server bind IP address")
	BindPORT = flag.Int("port", 8080, "server port")
	Display  = flag.String("display", ":0", "X11 display to use (e.g., :0, :1, :2)")
	DPI      = flag.Float64("dpi", 72, "screen resolution in Dots Per Inch")
	Fontfile = flag.String("fontfile", "internal/config/assets/fonts/JetBrainsMono-Regular.ttf", "filename of the ttf font")
	Hinting  = flag.String("hinting", "none", "none | full")
	Size     = flag.Float64("size", 9, "font size in points")
	Spacing  = flag.Float64("spacing", 1.5, "line spacing (e.g. 2 means double spaced)")
	Wonb     = flag.Bool("whiteonblack", false, "white text on a black background")

	// LLM Configuration (CLI args)
	Provider = flag.String("provider", "", "LLM provider to use (deepseek, zai, gemini, openai, anthropic, qwen, minimax)")
	APIKey   = flag.String("key", "", "LLM API key")
	Model    = flag.String("model", "", "LLM model name")
	BaseURL  = flag.String("base-url", "", "LLM base URL (optional, uses provider default if not specified)")

	// Persistent storage and config file
	ConfigPath  = flag.String("config", "", "Path to JSON config file (optional)")
	DataDir     = flag.String("data-dir", "", "Persistent data directory for config, logs, and artifacts")
	SessionLogs = flag.Bool("session-logs", true, "Enable writing streamed session logs to persistent storage")
)

// ProviderCredentials holds credentials and model list for a single provider
type ProviderCredentials struct {
	APIKey  string   `json:"apiKey"`
	BaseURL string   `json:"baseUrl,omitempty"`
	Models  []string `json:"models,omitempty"`
}

// LoggingConfig controls persistent session logging
type LoggingConfig struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Dir     string `json:"dir,omitempty"`
}

// ArtifactsConfig controls persistent artifact storage
type ArtifactsConfig struct {
	Dir string `json:"dir,omitempty"`
}

// FileConfig represents the optional JSON configuration file
type FileConfig struct {
	DefaultProvider string                         `json:"defaultProvider,omitempty"`
	DefaultModel    string                         `json:"defaultModel,omitempty"`
	Logging         LoggingConfig                  `json:"logging,omitempty"`
	Artifacts       ArtifactsConfig                `json:"artifacts,omitempty"`
	Providers       map[string]ProviderCredentials `json:"providers,omitempty"`
}

// ModelInfo describes a model and whether its provider has credentials configured
type ModelInfo struct {
	ID                   string   `json:"id"`
	Provider             string   `json:"provider"`
	HasCredentials       bool     `json:"hasCredentials"`
	ThinkingLevels       []string `json:"thinkingLevels,omitempty"`
	DefaultThinkingLevel string   `json:"defaultThinkingLevel,omitempty"`
}

// ThinkingLevelsForModel returns the supported thinking/reasoning levels (lowest to highest)
// for a thinking model, or nil if the model is not a thinking model.
func ThinkingLevelsForModel(model string) []string {
	switch model {
	case "gemini-4-neon", "gemini-2.5-pro":
		return []string{"low", "medium", "high"}
	case "gemini-2.5-flash":
		return []string{"none", "low", "medium", "high"}
	case "gpt-6.2-sol", "o3-mini":
		return []string{"low", "medium", "high"}
	case "claude-sonnet-5-5":
		return []string{"low", "medium", "high"}
	case "claude-opus-4-6":
		return []string{"low", "medium", "high", "max"}
	case "deepseek-reasoner":
		return []string{"low", "medium", "high"}
	case "glm-5.3":
		return []string{"low", "medium", "high"}
	case "qwen4-max":
		return []string{"low", "medium", "high"}
	case "MiniMax-M3.1":
		return []string{"low", "medium", "high"}
	default:
		return nil
	}
}

// LLMConfig holds the active/default LLM configuration
type LLMConfig struct {
	Provider string
	APIKey   string
	Model    string
	BaseURL  string
}

type runtimeConfig struct {
	mu              sync.RWMutex
	defaultProvider string
	defaultModel    string
	loggingEnabled  bool
	logsDir         string
	artifactsDir    string
	dataDir         string
	providers       map[string]ProviderCredentials
	providerOrder   []string
}

var rt = &runtimeConfig{
	loggingEnabled: true,
	providers:      make(map[string]ProviderCredentials),
	providerOrder:  []string{"deepseek", "zai", "gemini", "openai", "anthropic", "qwen", "minimax"},
}

func defaultProviderCatalog() map[string]ProviderCredentials {
	return map[string]ProviderCredentials{
		"deepseek": {
			BaseURL: "https://api.deepseek.com/v1",
			Models:  []string{"deepseek-flash", "deepseek-chat", "deepseek-reasoner"},
		},
		"zai": {
			BaseURL: "https://api.z.ai/api/paas/v4",
			Models:  []string{"glm-5.3", "glm-5.3-flash"},
		},
		"gemini": {
			BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/",
			Models:  []string{"gemini-4-neon", "gemini-2.5-pro", "gemini-2.5-flash"},
		},
		"openai": {
			BaseURL: "https://api.openai.com/v1",
			Models:  []string{"gpt-6.2-sol", "gpt-4o", "o3-mini"},
		},
		"anthropic": {
			BaseURL: "https://api.anthropic.com/v1/",
			Models:  []string{"claude-sonnet-5-5", "claude-opus-4-6"},
		},
		"qwen": {
			BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
			Models:  []string{"qwen4-max", "qwen-vl-max"},
		},
		"minimax": {
			BaseURL: "https://api.minimax.io/v1",
			Models:  []string{"MiniMax-M3.1-Flash", "MiniMax-M3.1"},
		},
	}
}

// LoadConfig loads the optional JSON config file and merges CLI arguments.
func LoadConfig() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.providers = defaultProviderCatalog()

	// Determine data directory
	explicitDataDir := false
	dataDir := *DataDir
	if dataDir != "" {
		explicitDataDir = true
	} else if envDir := os.Getenv("DATA_DIR"); envDir != "" {
		dataDir = envDir
		explicitDataDir = true
	} else {
		dataDir = "./data"
	}
	rt.dataDir = dataDir
	rt.logsDir = filepath.Join(dataDir, "logs")
	rt.artifactsDir = filepath.Join(dataDir, "artifacts")
	rt.loggingEnabled = *SessionLogs

	// Auto-bootstrap data directory (logs, artifacts, starter config.json) only if
	// explicitly configured or running inside a useless-agent directory. Never overwrites existing data.
	defaultCfgPath := filepath.Join(dataDir, "config.json")
	canBootstrap := explicitDataDir || isInsideUselessAgentDir()

	// Determine config file path
	cfgPath := *ConfigPath
	if cfgPath == "" {
		if envCfg := os.Getenv("CONFIG_PATH"); envCfg != "" {
			cfgPath = envCfg
		} else {
			cfgPath = defaultCfgPath
		}
	}

	if canBootstrap {
		bootstrapDataDir(dataDir, rt.logsDir, rt.artifactsDir, cfgPath, defaultCfgPath)
	}

	// Load optional config file if present
	if cfgPath != "" {
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("failed to read config file %s: %w", cfgPath, err)
			}
			log.Printf("Config file %s not found; continuing with defaults/CLI flags", cfgPath)
		} else {
			var fc FileConfig
			if err := json.Unmarshal(data, &fc); err != nil {
				return fmt.Errorf("failed to parse config file %s: %w", cfgPath, err)
			}
			log.Printf("Loaded config file: %s", cfgPath)

			if fc.DefaultProvider != "" {
				rt.defaultProvider = fc.DefaultProvider
			}
			if fc.DefaultModel != "" {
				rt.defaultModel = fc.DefaultModel
			}
			if fc.Logging.Enabled != nil {
				rt.loggingEnabled = *fc.Logging.Enabled
			}
			if fc.Logging.Dir != "" {
				rt.logsDir = fc.Logging.Dir
			}
			if fc.Artifacts.Dir != "" {
				rt.artifactsDir = fc.Artifacts.Dir
			}

			for name, pcfg := range fc.Providers {
				existing, exists := rt.providers[name]
				if !exists {
					rt.providerOrder = append(rt.providerOrder, name)
				}
				if pcfg.APIKey != "" {
					existing.APIKey = pcfg.APIKey
				}
				if pcfg.BaseURL != "" {
					existing.BaseURL = pcfg.BaseURL
				}
				if len(pcfg.Models) > 0 {
					existing.Models = pcfg.Models
				}
				rt.providers[name] = existing
			}
		}
	}

	// Merge CLI flags (CLI args take precedence for the specified provider)
	cliProvider := *Provider
	if cliProvider == "" && (*APIKey != "" || *Model != "" || *BaseURL != "") {
		cliProvider = "deepseek"
	}
	if cliProvider != "" {
		existing, exists := rt.providers[cliProvider]
		if !exists {
			rt.providerOrder = append(rt.providerOrder, cliProvider)
		}
		if *APIKey != "" {
			existing.APIKey = *APIKey
		}
		if *BaseURL != "" {
			existing.BaseURL = *BaseURL
		}
		if *Model != "" {
			found := false
			for _, m := range existing.Models {
				if m == *Model {
					found = true
					break
				}
			}
			if !found {
				existing.Models = append([]string{*Model}, existing.Models...)
			}
			rt.defaultModel = *Model
		}
		rt.providers[cliProvider] = existing
		rt.defaultProvider = cliProvider
	}

	// Pick sensible default provider/model if still unset
	if rt.defaultProvider == "" {
		// Prefer first provider that has credentials
		for _, name := range rt.providerOrder {
			if p, ok := rt.providers[name]; ok && p.APIKey != "" {
				rt.defaultProvider = name
				break
			}
		}
		if rt.defaultProvider == "" {
			rt.defaultProvider = "deepseek"
		}
	}

	if rt.defaultModel == "" {
		if p, ok := rt.providers[rt.defaultProvider]; ok && len(p.Models) > 0 {
			rt.defaultModel = p.Models[0]
		}
	}

	return nil
}

// GetLLMConfig returns the current default LLM configuration
func GetLLMConfig() LLMConfig {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	p := rt.providers[rt.defaultProvider]
	return LLMConfig{
		Provider: rt.defaultProvider,
		APIKey:   p.APIKey,
		Model:    rt.defaultModel,
		BaseURL:  p.BaseURL,
	}
}

// GetProviderConfig returns credentials/config for a specific provider
func GetProviderConfig(provider string) (ProviderCredentials, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	p, ok := rt.providers[provider]
	return p, ok
}

// ResolveProviderForModel finds the provider name for a given model ID
func ResolveProviderForModel(model string) (string, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	for _, name := range rt.providerOrder {
		p := rt.providers[name]
		for _, m := range p.Models {
			if m == model {
				return name, true
			}
		}
	}
	return "", false
}

// GetDefaultProviderAndModel returns the default provider and model
func GetDefaultProviderAndModel() (string, string) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.defaultProvider, rt.defaultModel
}

// GetAvailableModels returns all known models with their credential status,
// with models that have credentials listed first.
func GetAvailableModels() []ModelInfo {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	withCreds := make([]ModelInfo, 0)
	withoutCreds := make([]ModelInfo, 0)
	for _, name := range rt.providerOrder {
		p, ok := rt.providers[name]
		if !ok {
			continue
		}
		hasCreds := p.APIKey != ""
		for _, m := range p.Models {
			levels := ThinkingLevelsForModel(m)
			defaultLevel := ""
			if len(levels) > 0 {
				defaultLevel = levels[0] // Always default to lowest thinking level
			}
			info := ModelInfo{
				ID:                   m,
				Provider:             name,
				HasCredentials:       hasCreds,
				ThinkingLevels:       levels,
				DefaultThinkingLevel: defaultLevel,
			}
			if hasCreds {
				withCreds = append(withCreds, info)
			} else {
				withoutCreds = append(withoutCreds, info)
			}
		}
	}
	return append(withCreds, withoutCreds...)
}

// IsSessionLoggingEnabled returns whether persistent session log files are enabled
func IsSessionLoggingEnabled() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.loggingEnabled
}

// GetLogsDir returns the directory for persistent session logs
func GetLogsDir() string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.logsDir
}

// GetArtifactsDir returns the directory for persistent task artifacts
func GetArtifactsDir() string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.artifactsDir
}

// isInsideUselessAgentDir returns true if the current working directory is
// named "useless-agent" or is a subdirectory inside a "useless-agent" directory.
func isInsideUselessAgentDir() bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.Clean(cwd), string(filepath.Separator))
	for _, part := range parts {
		if part == "useless-agent" {
			return true
		}
	}
	return false
}

// bootstrapDataDir ensures the data directory, logs/, artifacts/, and a starter
// config.json exist without ever overwriting or destroying existing files.
func bootstrapDataDir(dataDir, logsDir, artifactsDir, cfgPath, defaultCfgPath string) {
	for _, dir := range []string{dataDir, logsDir, artifactsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("Warning: failed to create directory %s: %v", dir, err)
			return
		}
	}

	if filepath.Clean(cfgPath) != filepath.Clean(defaultCfgPath) {
		return
	}

	f, err := os.OpenFile(cfgPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if !os.IsExist(err) {
			log.Printf("Warning: failed to create starter config %s: %v", cfgPath, err)
		}
		return
	}
	defer f.Close()

	enabled := true
	starter := FileConfig{
		DefaultProvider: "deepseek",
		DefaultModel:    "deepseek-flash",
		Logging: LoggingConfig{
			Enabled: &enabled,
			Dir:     logsDir,
		},
		Artifacts: ArtifactsConfig{
			Dir: artifactsDir,
		},
		Providers: defaultProviderCatalog(),
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(starter); err != nil {
		log.Printf("Warning: failed to write starter config %s: %v", cfgPath, err)
		return
	}
	log.Printf("Created starter config at %s — add your provider API keys there or pass --provider/--key", cfgPath)
}
