package http

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"useless-agent/internal/storage"
)

// maxRecordingUploadBytes caps a single recording upload (2 GiB).
const maxRecordingUploadBytes = 2 << 30

// RecordingsHandler lists (GET) and uploads (POST) session recordings.
// Uploads are multipart with fields: file, startedAt (RFC3339), sessionIp, sessionId.
func RecordingsHandler(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		recordings, err := storage.ListSessionRecordings()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to list recordings: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"recordings": recordings})

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, maxRecordingUploadBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Expected multipart form upload: "+err.Error())
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Missing 'file' field: "+err.Error())
			return
		}
		defer file.Close()

		startedAt := time.Now()
		if raw := strings.TrimSpace(r.FormValue("startedAt")); raw != "" {
			parsed, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "Invalid startedAt (RFC3339 expected)")
				return
			}
			startedAt = parsed
		}

		session := strings.TrimSpace(r.FormValue("sessionIp"))
		if session == "" {
			session = strings.TrimSpace(r.FormValue("sessionId"))
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")

		info, err := storage.SaveSessionRecording(startedAt, session, ext, file)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to save recording: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, info)

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
