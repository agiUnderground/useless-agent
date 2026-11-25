package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	ErrUserExists       = errors.New("username already exists")
	ErrInvalidCreds     = errors.New("invalid username or password")
	ErrNotFound         = errors.New("not found")
	ErrCannotDeleteLast = errors.New("cannot delete default or last workspace")
)

const SessionTTL = 7 * 24 * time.Hour

type User struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	ActiveWorkspaceID string `json:"activeWorkspaceId,omitempty"`
	CreatedAt         int64  `json:"createdAt"`
}

type AuthSession struct {
	Token     string `json:"token"`
	UserID    string `json:"userId"`
	ExpiresAt int64  `json:"expiresAt"` // Unix ms
	CreatedAt int64  `json:"createdAt"` // Unix ms
}

type WorkspaceSession struct {
	ID        string `json:"id"`
	IP        string `json:"ip"`
	Name      string `json:"name"`
	Selected  bool   `json:"selected"`
	Maximized bool   `json:"maximized"`
}

type WorkspaceTask struct {
	ID             string `json:"id"`
	SessionID      string `json:"sessionId,omitempty"`
	SessionIP      string `json:"sessionIp,omitempty"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	Model          string `json:"model,omitempty"`
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`
	SequenceNumber int    `json:"sequenceNumber,omitempty"`
	CreatedAt      int64  `json:"createdAt,omitempty"`
	CompletedAt    int64  `json:"completedAt,omitempty"`
}

type WorkspaceWithState struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	IsDefault    bool               `json:"isDefault"`
	FPS          int                `json:"fps"`
	TokenUsage   int                `json:"tokenUsage"`
	NetworkUsage int64              `json:"networkUsage"`
	CreatedAt    int64              `json:"createdAt"`
	UpdatedAt    int64              `json:"updatedAt"`
	Sessions     []WorkspaceSession `json:"sessions"`
	Tasks        []WorkspaceTask    `json:"tasks"`
}

type InventoryServer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IP          string `json:"ip"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	// IsDisposable marks a row backed by a temporary Docker container that is
	// destroyed together with the row.
	IsDisposable bool   `json:"isDisposable"`
	ContainerID  string `json:"containerId,omitempty"`
}

// inventoryColumns lists the inventory_servers columns in scan order.
const inventoryColumns = `id, name, ip, description, created_at, updated_at, is_disposable, container_id`

func scanInventory(row interface{ Scan(dest ...any) error }) (*InventoryServer, error) {
	var s InventoryServer
	err := row.Scan(&s.ID, &s.Name, &s.IP, &s.Description, &s.CreatedAt, &s.UpdatedAt, &s.IsDisposable, &s.ContainerID)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

var (
	dbConn *sql.DB
	dbMu   sync.RWMutex
)

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// InitDB opens the SQLite database at dbPath and ensures all tables exist.
func InitDB(dbPath string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	if dbConn != nil {
		_ = dbConn.Close()
		dbConn = nil
	}

	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create db dir: %w", err)
		}
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite works best with a single writer connection in-process.
	conn.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			_ = conn.Close()
			return fmt.Errorf("exec pragma %q: %w", p, err)
		}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		created_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS auth_sessions (
		token TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		expires_at INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		is_default INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS workspace_sessions (
		id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		ip TEXT NOT NULL,
		name TEXT NOT NULL,
		selected INTEGER NOT NULL DEFAULT 0,
		maximized INTEGER NOT NULL DEFAULT 0,
		sort_order INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, id),
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS workspace_tasks (
		id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		session_id TEXT NOT NULL DEFAULT '',
		session_ip TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		message TEXT NOT NULL,
		model TEXT NOT NULL DEFAULT '',
		thinking_level TEXT NOT NULL DEFAULT '',
		sequence_number INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL DEFAULT 0,
		completed_at INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, id),
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS inventory_servers (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		ip TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		is_disposable INTEGER NOT NULL DEFAULT 0,
		container_id TEXT NOT NULL DEFAULT '',
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	`
	if _, err := conn.Exec(schema); err != nil {
		_ = conn.Close()
		return fmt.Errorf("init schema: %w", err)
	}

	// Safe migrations for existing databases
	migrations := []string{
		`ALTER TABLE users ADD COLUMN active_workspace_id TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE workspaces ADD COLUMN fps INTEGER NOT NULL DEFAULT 5;`,
		`ALTER TABLE workspaces ADD COLUMN token_usage INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE workspaces ADD COLUMN network_usage INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE inventory_servers ADD COLUMN is_disposable INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE inventory_servers ADD COLUMN container_id TEXT NOT NULL DEFAULT '';`,
	}
	for _, m := range migrations {
		_, _ = conn.Exec(m) // ignore "duplicate column name" errors
	}

	dbConn = conn
	return nil
}

func getDB() (*sql.DB, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()
	if dbConn == nil {
		return nil, errors.New("database not initialized")
	}
	return dbConn, nil
}

// RegisterUser creates a new user with a salted bcrypt password hash and a Default workspace.
func RegisterUser(username, password string) (*User, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	if len(username) < 2 || len(username) > 64 {
		return nil, errors.New("username must be between 2 and 64 characters")
	}
	if len(password) < 4 {
		return nil, errors.New("password must be at least 4 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now().UnixMilli()
	user := &User{
		ID:        newID("usr"),
		Username:  username,
		CreatedAt: now,
	}

	tx, err := conn.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		user.ID, user.Username, string(hash), user.CreatedAt,
	); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrUserExists
		}
		return nil, err
	}

	// Create Default workspace for the new user
	wsID := newID("ws")
	if _, err := tx.Exec(
		`INSERT INTO workspaces (id, user_id, name, is_default, created_at, updated_at) VALUES (?, ?, 'Default', 1, ?, ?)`,
		wsID, user.ID, now, now,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return user, nil
}

// AuthenticateUser verifies username and password against the salted bcrypt hash.
func AuthenticateUser(username, password string) (*User, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	var user User
	var hash string
	err = conn.QueryRow(
		`SELECT id, username, password_hash, active_workspace_id, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &hash, &user.ActiveWorkspaceID, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCreds
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return &user, nil
}

// CreateAuthSession creates a new persisted session token with the given TTL.
func CreateAuthSession(userID string, ttl time.Duration) (*AuthSession, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &AuthSession{
		Token:     token,
		UserID:    userID,
		CreatedAt: now.UnixMilli(),
		ExpiresAt: now.Add(ttl).UnixMilli(),
	}
	if _, err := conn.Exec(
		`INSERT INTO auth_sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		sess.Token, sess.UserID, sess.ExpiresAt, sess.CreatedAt,
	); err != nil {
		return nil, err
	}
	return sess, nil
}

// ValidateAuthSession checks whether a token exists and has not expired.
func ValidateAuthSession(token string) (*User, *AuthSession, error) {
	conn, err := getDB()
	if err != nil {
		return nil, nil, err
	}
	if token == "" {
		return nil, nil, ErrInvalidCreds
	}

	var user User
	var sess AuthSession
	err = conn.QueryRow(`
		SELECT s.token, s.user_id, s.expires_at, s.created_at, u.id, u.username, u.active_workspace_id, u.created_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ?
	`, token).Scan(&sess.Token, &sess.UserID, &sess.ExpiresAt, &sess.CreatedAt, &user.ID, &user.Username, &user.ActiveWorkspaceID, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrInvalidCreds
		}
		return nil, nil, err
	}

	if time.Now().UnixMilli() > sess.ExpiresAt {
		_, _ = conn.Exec(`DELETE FROM auth_sessions WHERE token = ?`, token)
		return nil, nil, ErrInvalidCreds
	}
	return &user, &sess, nil
}

// DeleteAuthSession revokes a session token.
func DeleteAuthSession(token string) error {
	conn, err := getDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(`DELETE FROM auth_sessions WHERE token = ?`, token)
	return err
}

// SetActiveWorkspace persists the user's last selected workspace ID.
func SetActiveWorkspace(userID, workspaceID string) error {
	conn, err := getDB()
	if err != nil {
		return err
	}
	_, err = conn.Exec(`UPDATE users SET active_workspace_id = ? WHERE id = ?`, workspaceID, userID)
	return err
}

// ListWorkspaces returns all workspaces (with sessions & tasks) for a user, ensuring Default exists.
func ListWorkspaces(userID string) ([]WorkspaceWithState, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}

	rows, err := conn.Query(`
		SELECT id, name, is_default, fps, token_usage, network_usage, created_at, updated_at
		FROM workspaces
		WHERE user_id = ?
		ORDER BY is_default DESC, created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}

	workspaces := make([]WorkspaceWithState, 0)
	for rows.Next() {
		var ws WorkspaceWithState
		var isDef int
		if err := rows.Scan(&ws.ID, &ws.Name, &isDef, &ws.FPS, &ws.TokenUsage, &ws.NetworkUsage, &ws.CreatedAt, &ws.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if ws.FPS <= 0 {
			ws.FPS = 5
		}
		ws.IsDefault = isDef != 0
		ws.Sessions = make([]WorkspaceSession, 0)
		ws.Tasks = make([]WorkspaceTask, 0)
		workspaces = append(workspaces, ws)
	}
	_ = rows.Close()

	// Ensure Default workspace always exists
	if len(workspaces) == 0 {
		now := time.Now().UnixMilli()
		ws := WorkspaceWithState{
			ID:        newID("ws"),
			Name:      "Default",
			IsDefault: true,
			FPS:       5,
			CreatedAt: now,
			UpdatedAt: now,
			Sessions:  make([]WorkspaceSession, 0),
			Tasks:     make([]WorkspaceTask, 0),
		}
		if _, err := conn.Exec(
			`INSERT INTO workspaces (id, user_id, name, is_default, fps, token_usage, network_usage, created_at, updated_at) VALUES (?, ?, 'Default', 1, 5, 0, 0, ?, ?)`,
			ws.ID, userID, now, now,
		); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}

	for i := range workspaces {
		wsID := workspaces[i].ID
		sRows, err := conn.Query(`
			SELECT id, ip, name, selected, maximized
			FROM workspace_sessions
			WHERE workspace_id = ?
			ORDER BY sort_order ASC
		`, wsID)
		if err != nil {
			return nil, err
		}
		for sRows.Next() {
			var s WorkspaceSession
			var sel, max int
			if err := sRows.Scan(&s.ID, &s.IP, &s.Name, &sel, &max); err != nil {
				_ = sRows.Close()
				return nil, err
			}
			s.Selected = sel != 0
			s.Maximized = max != 0
			workspaces[i].Sessions = append(workspaces[i].Sessions, s)
		}
		_ = sRows.Close()

		tRows, err := conn.Query(`
			SELECT id, session_id, session_ip, status, message, model, thinking_level, sequence_number, created_at, completed_at
			FROM workspace_tasks
			WHERE workspace_id = ?
			ORDER BY sequence_number ASC, created_at ASC
		`, wsID)
		if err != nil {
			return nil, err
		}
		for tRows.Next() {
			var t WorkspaceTask
			if err := tRows.Scan(
				&t.ID, &t.SessionID, &t.SessionIP, &t.Status, &t.Message,
				&t.Model, &t.ThinkingLevel, &t.SequenceNumber, &t.CreatedAt, &t.CompletedAt,
			); err != nil {
				_ = tRows.Close()
				return nil, err
			}
			workspaces[i].Tasks = append(workspaces[i].Tasks, t)
		}
		_ = tRows.Close()
	}

	return workspaces, nil
}

// CreateWorkspace creates a new workspace for a user.
func CreateWorkspace(userID, name string) (*WorkspaceWithState, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("workspace name is required")
	}
	now := time.Now().UnixMilli()
	ws := &WorkspaceWithState{
		ID:        newID("ws"),
		Name:      name,
		IsDefault: false,
		FPS:       5,
		CreatedAt: now,
		UpdatedAt: now,
		Sessions:  make([]WorkspaceSession, 0),
		Tasks:     make([]WorkspaceTask, 0),
	}
	if _, err := conn.Exec(
		`INSERT INTO workspaces (id, user_id, name, is_default, fps, token_usage, network_usage, created_at, updated_at) VALUES (?, ?, ?, 0, 5, 0, 0, ?, ?)`,
		ws.ID, userID, ws.Name, now, now,
	); err != nil {
		return nil, err
	}
	return ws, nil
}

// RenameWorkspace renames a workspace owned by userID.
func RenameWorkspace(userID, workspaceID, name string) error {
	conn, err := getDB()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("workspace name is required")
	}
	res, err := conn.Exec(
		`UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
		name, time.Now().UnixMilli(), workspaceID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteWorkspace deletes a non-default workspace owned by userID.
func DeleteWorkspace(userID, workspaceID string) error {
	conn, err := getDB()
	if err != nil {
		return err
	}
	var isDef int
	err = conn.QueryRow(`SELECT is_default FROM workspaces WHERE id = ? AND user_id = ?`, workspaceID, userID).Scan(&isDef)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if isDef != 0 {
		return ErrCannotDeleteLast
	}
	_, err = conn.Exec(`DELETE FROM workspaces WHERE id = ? AND user_id = ?`, workspaceID, userID)
	return err
}

// SaveWorkspaceState atomically replaces the sessions, tasks, and workspace metrics/settings.
func SaveWorkspaceState(userID, workspaceID string, sessions []WorkspaceSession, tasks []WorkspaceTask, fps int, tokenUsage int, networkUsage int64) error {
	conn, err := getDB()
	if err != nil {
		return err
	}
	if fps <= 0 {
		fps = 5
	}

	var exists int
	if err := conn.QueryRow(`SELECT 1 FROM workspaces WHERE id = ? AND user_id = ?`, workspaceID, userID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM workspace_sessions WHERE workspace_id = ?`, workspaceID); err != nil {
		return err
	}
	for idx, s := range sessions {
		sel, max := 0, 0
		if s.Selected {
			sel = 1
		}
		if s.Maximized {
			max = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO workspace_sessions (id, workspace_id, ip, name, selected, maximized, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, s.ID, workspaceID, s.IP, s.Name, sel, max, idx); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM workspace_tasks WHERE workspace_id = ?`, workspaceID); err != nil {
		return err
	}
	for _, t := range tasks {
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO workspace_tasks
			(id, workspace_id, session_id, session_ip, status, message, model, thinking_level, sequence_number, created_at, completed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, t.ID, workspaceID, t.SessionID, t.SessionIP, t.Status, t.Message, t.Model, t.ThinkingLevel, t.SequenceNumber, t.CreatedAt, t.CompletedAt); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		`UPDATE workspaces SET fps = ?, token_usage = ?, network_usage = ?, updated_at = ? WHERE id = ?`,
		fps, tokenUsage, networkUsage, time.Now().UnixMilli(), workspaceID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// ListInventory returns all inventory servers for userID.
func ListInventory(userID string) ([]InventoryServer, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(`
		SELECT `+inventoryColumns+`
		FROM inventory_servers
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	servers := make([]InventoryServer, 0)
	for rows.Next() {
		s, err := scanInventory(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, *s)
	}
	return servers, rows.Err()
}

// CreateInventoryServer adds a server to the user's inventory.
func CreateInventoryServer(userID, name, ip, description string) (*InventoryServer, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	ip = strings.TrimSpace(ip)
	description = strings.TrimSpace(description)
	if ip == "" {
		return nil, errors.New("server IP is required")
	}
	if name == "" {
		name = ip
	}
	now := time.Now().UnixMilli()
	s := &InventoryServer{
		ID:          newID("srv"),
		Name:        name,
		IP:          ip,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := conn.Exec(`
		INSERT INTO inventory_servers (id, user_id, name, ip, description, created_at, updated_at, is_disposable, container_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, '')
	`, s.ID, userID, s.Name, s.IP, s.Description, s.CreatedAt, s.UpdatedAt); err != nil {
		return nil, err
	}
	return s, nil
}

// CreateDisposableInventoryServer adds an inventory row backed by a disposable
// Docker container. The container must already exist; containerID is what is
// destroyed when the row is deleted.
func CreateDisposableInventoryServer(userID, name, ip, description, containerID string) (*InventoryServer, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	s := &InventoryServer{
		ID:           newID("dsp"),
		Name:         strings.TrimSpace(name),
		IP:           strings.TrimSpace(ip),
		Description:  strings.TrimSpace(description),
		CreatedAt:    now,
		UpdatedAt:    now,
		IsDisposable: true,
		ContainerID:  strings.TrimSpace(containerID),
	}
	if _, err := conn.Exec(`
		INSERT INTO inventory_servers (id, user_id, name, ip, description, created_at, updated_at, is_disposable, container_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, s.ID, userID, s.Name, s.IP, s.Description, s.CreatedAt, s.UpdatedAt, s.ContainerID); err != nil {
		return nil, err
	}
	return s, nil
}

// GetInventoryServer returns one inventory server owned by userID.
func GetInventoryServer(userID, id string) (*InventoryServer, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	s, err := scanInventory(conn.QueryRow(`
		SELECT `+inventoryColumns+`
		FROM inventory_servers WHERE id = ? AND user_id = ?
	`, id, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpdateInventoryServer updates an existing inventory server owned by userID.
func UpdateInventoryServer(userID, id, name, ip, description string) (*InventoryServer, error) {
	conn, err := getDB()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	ip = strings.TrimSpace(ip)
	description = strings.TrimSpace(description)
	if ip == "" {
		return nil, errors.New("server IP is required")
	}
	if name == "" {
		name = ip
	}
	now := time.Now().UnixMilli()
	res, err := conn.Exec(`
		UPDATE inventory_servers
		SET name = ?, ip = ?, description = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`, name, ip, description, now, id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	s, err := scanInventory(conn.QueryRow(`
		SELECT `+inventoryColumns+`
		FROM inventory_servers WHERE id = ? AND user_id = ?
	`, id, userID))
	if err != nil {
		return nil, err
	}
	return s, nil
}

// DeleteInventoryServer deletes an inventory server owned by userID.
func DeleteInventoryServer(userID, id string) error {
	conn, err := getDB()
	if err != nil {
		return err
	}
	res, err := conn.Exec(`DELETE FROM inventory_servers WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
