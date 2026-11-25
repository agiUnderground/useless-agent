package storage

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"useless-agent/internal/config"
)

// recordingsDirName is the sub-directory of the artifacts directory where
// session video recordings are stored.
const recordingsDirName = "session-recording"

// Allowed recording container extensions; anything else is stored as webm.
var recordingExts = map[string]bool{
	"webm": true,
	"mp4":  true,
	"mov":  true,
	"ogg":  true,
	"mkv":  true,
}

// GetSessionRecordingsDir returns (and creates) the recordings directory.
func GetSessionRecordingsDir() (string, error) {
	dir := filepath.Join(config.GetArtifactsDir(), recordingsDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create recordings directory %s: %w", dir, err)
	}
	return dir, nil
}

// RecordingInfo describes a saved session recording file.
type RecordingInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	StartedAt string `json:"startedAt"`
	UpdatedAt string `json:"updatedAt"`
}

// SaveSessionRecording stores an uploaded recording under
// <artifacts>/session-recording. The file name starts with the recording start
// time (UTC) so files sort chronologically and stay unique; a numeric suffix is
// added if two recordings started in the same second.
func SaveSessionRecording(startedAt time.Time, session, ext string, r io.Reader) (*RecordingInfo, error) {
	dir, err := GetSessionRecordingsDir()
	if err != nil {
		return nil, err
	}

	if !recordingExts[ext] {
		ext = "webm"
	}
	label := sanitizeForFilename(session)
	stamp := startedAt.UTC().Format("20060102_150405")

	dst := filepath.Join(dir, fmt.Sprintf("%s_%s.%s", stamp, label, ext))
	for i := 1; ; i++ {
		if _, err := os.Stat(dst); err != nil {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("%s_%s-%02d.%s", stamp, label, i, ext))
	}

	f, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	rel, _ := filepath.Rel(config.GetArtifactsDir(), dst)
	return &RecordingInfo{
		Name:      filepath.Base(dst),
		Path:      filepath.ToSlash(rel),
		Size:      n,
		StartedAt: startedAt.UTC().Format(time.RFC3339),
		UpdatedAt: now.Format(time.RFC3339),
	}, nil
}

// ListSessionRecordings lists saved recordings, newest first.
func ListSessionRecordings() ([]RecordingInfo, error) {
	dir := filepath.Join(config.GetArtifactsDir(), recordingsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RecordingInfo{}, nil
		}
		log.Printf("Error reading recordings dir: %v", err)
		return nil, err
	}

	results := make([]RecordingInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		results = append(results, RecordingInfo{
			Name:      entry.Name(),
			Path:      recordingsDirName + "/" + entry.Name(),
			Size:      info.Size(),
			StartedAt: parseRecordingStart(entry.Name()),
			UpdatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.Compare(results[i].Name, results[j].Name) > 0
	})
	return results, nil
}

// parseRecordingStart extracts the leading UTC timestamp of a recording file name.
func parseRecordingStart(name string) string {
	if len(name) < 15 {
		return ""
	}
	t, err := time.ParseInLocation("20060102_150405", name[:15], time.UTC)
	if err != nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
