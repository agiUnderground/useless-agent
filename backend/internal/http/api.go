package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"useless-agent/internal/db"
	"useless-agent/internal/disposable"
)

const authCookieName = "ua_session"

// disposables is the docker-controller client; nil or unconfigured means the
// feature is disabled for this deployment.
var disposables = disposable.FromEnv()

// SetDisposableClient overrides the docker-controller client (used by tests).
func SetDisposableClient(c *disposable.Client) { disposables = c }

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func extractAuthToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			return strings.TrimSpace(auth[7:])
		}
	}
	if c, err := r.Cookie(authCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(db.SessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func requireAuth(w http.ResponseWriter, r *http.Request) (*db.User, *db.AuthSession, bool) {
	token := extractAuthToken(r)
	user, sess, err := db.ValidateAuthSession(token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return nil, nil, false
	}
	return user, sess, true
}

// AuthRegisterHandler handles POST /api/auth/register
func AuthRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	user, err := db.RegisterUser(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, db.ErrUserExists) {
			writeJSONError(w, http.StatusConflict, "Username already exists")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	sess, err := db.CreateAuthSession(user.ID, db.SessionTTL)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	setAuthCookie(w, sess.Token)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":     sess.Token,
		"expiresAt": sess.ExpiresAt,
		"user":      user,
	})
}

// AuthLoginHandler handles POST /api/auth/login
func AuthLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	user, err := db.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	sess, err := db.CreateAuthSession(user.ID, db.SessionTTL)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	setAuthCookie(w, sess.Token)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":     sess.Token,
		"expiresAt": sess.ExpiresAt,
		"user":      user,
	})
}

// AuthMeHandler handles GET /api/auth/me
func AuthMeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	user, sess, ok := requireAuth(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":      user,
		"expiresAt": sess.ExpiresAt,
	})
}

// AuthLogoutHandler handles POST /api/auth/logout
func AuthLogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if token := extractAuthToken(r); token != "" {
		_ = db.DeleteAuthSession(token)
	}
	clearAuthCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"result": "logged out"})
}

// UserPreferencesHandler handles PUT /api/user/preferences
func UserPreferencesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	user, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	var req struct {
		ActiveWorkspaceID string `json:"activeWorkspaceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := db.SetActiveWorkspace(user.ID, req.ActiveWorkspaceID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "saved"})
}

// WorkspacesHandler handles GET and POST /api/workspaces
func WorkspacesHandler(w http.ResponseWriter, r *http.Request) {
	user, _, ok := requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		workspaces, err := db.ListWorkspaces(user.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"workspaces": workspaces})

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		ws, err := db.CreateWorkspace(user.ID, req.Name)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, ws)

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// WorkspaceByIDHandler handles /api/workspaces/{id} and /api/workspaces/{id}/state
func WorkspaceByIDHandler(w http.ResponseWriter, r *http.Request) {
	user, _, ok := requireAuth(w, r)
	if !ok {
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/workspaces/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSONError(w, http.StatusNotFound, "Workspace ID required")
		return
	}
	wsID := parts[0]

	// /api/workspaces/{id}/state
	if len(parts) == 2 && parts[1] == "state" {
		if r.Method != http.MethodPut {
			writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		var req struct {
			Sessions     []db.WorkspaceSession `json:"sessions"`
			Tasks        []db.WorkspaceTask    `json:"tasks"`
			FPS          int                   `json:"fps"`
			TokenUsage   int                   `json:"tokenUsage"`
			NetworkUsage int64                 `json:"networkUsage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := db.SaveWorkspaceState(user.ID, wsID, req.Sessions, req.Tasks, req.FPS, req.TokenUsage, req.NetworkUsage); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "Workspace not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"result": "saved"})
		return
	}

	if len(parts) != 1 {
		writeJSONError(w, http.StatusNotFound, "Not found")
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		if err := db.RenameWorkspace(user.ID, wsID, req.Name); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "Workspace not found")
				return
			}
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"result": "updated"})

	case http.MethodDelete:
		if err := db.DeleteWorkspace(user.ID, wsID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "Workspace not found")
				return
			}
			if errors.Is(err, db.ErrCannotDeleteLast) {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// InventoryHandler handles GET and POST /api/inventory
func InventoryHandler(w http.ResponseWriter, r *http.Request) {
	user, _, ok := requireAuth(w, r)
	if !ok {
		return
	}

	switch r.Method {
	case http.MethodGet:
		servers, err := db.ListInventory(user.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"servers": servers})

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			IP          string `json:"ip"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		srv, err := db.CreateInventoryServer(user.ID, req.Name, req.IP, req.Description)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, srv)

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// InventoryByIDHandler handles PATCH and DELETE /api/inventory/{id}
func InventoryByIDHandler(w http.ResponseWriter, r *http.Request) {
	user, _, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/inventory/"), "/")
	if id == "" || strings.Contains(id, "/") {
		writeJSONError(w, http.StatusNotFound, "Server ID required")
		return
	}

	existing, err := db.GetInventoryServer(user.ID, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "Server not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	switch r.Method {
	case http.MethodPatch:
		if existing.IsDisposable {
			writeJSONError(w, http.StatusBadRequest, "Disposable containers cannot be edited. Destroy it and create a new one.")
			return
		}
		var req struct {
			Name        string `json:"name"`
			IP          string `json:"ip"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		srv, err := db.UpdateInventoryServer(user.ID, id, req.Name, req.IP, req.Description)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "Server not found")
				return
			}
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, srv)

	case http.MethodDelete:
		if existing.IsDisposable && existing.ContainerID != "" {
			// Destroy the container first. If that fails the row is kept so the
			// user can retry instead of leaking a running container.
			if err := destroyDisposable(r, existing.ContainerID); err != nil {
				writeJSONError(w, http.StatusBadGateway, "Could not destroy container: "+err.Error())
				return
			}
		}
		if err := db.DeleteInventoryServer(user.ID, id); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "Server not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"result": "deleted"})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// destroyDisposable removes a disposable container. A container that is
// already gone counts as success.
func destroyDisposable(r *http.Request, containerID string) error {
	if !disposables.Configured() {
		return disposable.ErrNotConfigured
	}
	err := disposables.Delete(r.Context(), containerID)
	var se *disposable.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return nil
	}
	return err
}

// DisposableCreateHandler handles POST /api/inventory/disposable: it creates a
// disposable Docker container and adds it to the user's inventory.
func DisposableCreateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	user, _, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if !disposables.Configured() {
		writeJSONError(w, http.StatusNotImplemented, "Disposable containers are not available in this deployment")
		return
	}

	c, err := disposables.Create(r.Context(), user.ID)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "Could not create container: "+err.Error())
		return
	}

	name := strings.TrimPrefix(c.Name, "uagent-disp-")
	srv, err := db.CreateDisposableInventoryServer(
		user.ID,
		"disposable-"+name,
		c.IP,
		"Temporary container "+c.Name+". Destroyed together with this entry.",
		c.ID,
	)
	if err != nil {
		// Do not leak a running container when the row cannot be stored.
		_ = disposables.Delete(context.Background(), c.ID)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}

// CapabilitiesHandler handles GET /api/capabilities and tells the frontend
// which optional features this deployment supports.
func CapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if _, _, ok := requireAuth(w, r); !ok {
		return
	}
	enabled, reason := false, ""
	if !disposables.Configured() {
		reason = "Disposable containers are only available when started with docker compose and a configured controller."
	} else if err := disposables.Health(r.Context()); err != nil {
		reason = "Container controller is unavailable: " + err.Error()
	} else {
		enabled = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"disposableContainers":       enabled,
		"disposableContainersReason": reason,
	})
}
