package storage

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"useless-agent/internal/config"
)

var (
	sessionLogMu      sync.Mutex
	activeLogFile     *os.File
	activeLogFileName string
	activeSessionID   string
	nonSafeChars      = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
)

// InitStorage creates persistent directories for logs and artifacts.
func InitStorage() error {
	if config.IsSessionLoggingEnabled() {
		if err := os.MkdirAll(config.GetLogsDir(), 0755); err != nil {
			return fmt.Errorf("failed to create logs directory %s: %w", config.GetLogsDir(), err)
		}
	}
	if err := os.MkdirAll(config.GetArtifactsDir(), 0755); err != nil {
		return fmt.Errorf("failed to create artifacts directory %s: %w", config.GetArtifactsDir(), err)
	}
	return nil
}

func sanitizeForFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = nonSafeChars.ReplaceAllString(s, "_")
	return s
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127-0-0-1"
	}
	var fallbackIPv6 string
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return sanitizeForFilename(ip4.String())
			}
			if fallbackIPv6 == "" && ipnet.IP.To16() != nil && !ipnet.IP.IsLinkLocalUnicast() {
				fallbackIPv6 = sanitizeForFilename(ipnet.IP.String())
			}
		}
	}
	if fallbackIPv6 != "" {
		return fallbackIPv6
	}
	return "127-0-0-1"
}

// StartSessionLog opens (or rotates to) a new identifiable session log file:
// <YYYYMMDD_HHMMSS>_<ip>_<sessionId>.log
func StartSessionLog(clientIP, sessionID string) {
	if !config.IsSessionLoggingEnabled() {
		return
	}

	sessionLogMu.Lock()
	defer sessionLogMu.Unlock()

	if sessionID == "" {
		sessionID = "default"
	}
	// If already logging for this session, keep the current file
	if activeLogFile != nil && activeSessionID == sessionID {
		return
	}

	if activeLogFile != nil {
		_ = activeLogFile.Close()
		activeLogFile = nil
	}

	ipPart := sanitizeForFilename(clientIP)
	if ipPart == "unknown" || ipPart == "" {
		ipPart = getLocalIP()
	}
	sessPart := sanitizeForFilename(sessionID)
	ts := time.Now().UTC().Format("20060102_150405")

	fileName := fmt.Sprintf("%s_%s_%s.log", ts, ipPart, sessPart)
	fullPath := filepath.Join(config.GetLogsDir(), fileName)

	if err := os.MkdirAll(config.GetLogsDir(), 0755); err != nil {
		return
	}

	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}

	activeLogFile = f
	activeLogFileName = fullPath
	activeSessionID = sessionID
}

// WriteSessionLog writes a streamed log line to the active session log file.
func WriteSessionLog(line string) {
	if !config.IsSessionLoggingEnabled() {
		return
	}

	sessionLogMu.Lock()
	defer sessionLogMu.Unlock()

	if activeLogFile == nil {
		// Lazily create a default session log file on first write
		ipPart := getLocalIP()
		ts := time.Now().UTC().Format("20060102_150405")
		fileName := fmt.Sprintf("%s_%s_server.log", ts, ipPart)
		fullPath := filepath.Join(config.GetLogsDir(), fileName)
		if err := os.MkdirAll(config.GetLogsDir(), 0755); err != nil {
			return
		}
		f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		activeLogFile = f
		activeLogFileName = fullPath
		activeSessionID = "server"
	}

	_, _ = activeLogFile.WriteString(line)
}

// GetTaskArtifactsDir returns (and ensures) the persistent artifacts directory for a task.
func GetTaskArtifactsDir(taskID string) (string, error) {
	base := config.GetArtifactsDir()
	dir := base
	if taskID != "" {
		dir = filepath.Join(base, sanitizeForFilename(taskID))
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// ArtifactInfo describes a saved artifact file.
type ArtifactInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	TaskID    string `json:"taskId,omitempty"`
	Size      int64  `json:"size"`
	UpdatedAt string `json:"updatedAt"`
}

// ListArtifacts lists all artifacts in the persistent artifacts directory (optionally filtered by taskID).
func ListArtifacts(taskID string) ([]ArtifactInfo, error) {
	base := config.GetArtifactsDir()
	searchRoot := base
	if taskID != "" {
		searchRoot = filepath.Join(base, sanitizeForFilename(taskID))
	}

	if _, err := os.Stat(searchRoot); os.IsNotExist(err) {
		return []ArtifactInfo{}, nil
	}

	var results []ArtifactInfo
	err := filepath.Walk(searchRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		parts := strings.SplitN(rel, "/", 2)
		tid := ""
		if len(parts) == 2 {
			tid = parts[0]
		}
		results = append(results, ArtifactInfo{
			Name:      info.Name(),
			Path:      rel,
			TaskID:    tid,
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		log.Printf("Error walking artifacts dir: %v", err)
		return nil, err
	}
	return results, nil
}

// SaveArtifact saves an artifact from an io.Reader into the persistent artifacts directory.
func SaveArtifact(taskID, fileName string, r io.Reader) (*ArtifactInfo, error) {
	cleanName := filepath.Base(fileName)
	if cleanName == "" || cleanName == "." || cleanName == "/" {
		return nil, fmt.Errorf("invalid artifact filename")
	}

	dir, err := GetTaskArtifactsDir(taskID)
	if err != nil {
		return nil, err
	}

	dstPath := filepath.Join(dir, cleanName)
	f, err := os.Create(dstPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		return nil, err
	}

	rel, _ := filepath.Rel(config.GetArtifactsDir(), dstPath)
	return &ArtifactInfo{
		Name:      cleanName,
		Path:      filepath.ToSlash(rel),
		TaskID:    taskID,
		Size:      n,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
