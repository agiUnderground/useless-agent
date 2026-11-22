package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceAndAuth(t *testing.T) {
	tmpDir := t.TempDir()
	if err := InitDB(filepath.Join(tmpDir, "test.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Register user (salted bcrypt) + Default workspace
	user, err := RegisterUser("alice", "secret123")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	if user.Username != "alice" {
		t.Fatalf("unexpected username: %s", user.Username)
	}

	// Duplicate registration must fail
	if _, err := RegisterUser("alice", "otherpass"); err != ErrUserExists {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}

	// Wrong password must fail
	if _, err := AuthenticateUser("alice", "wrongpass"); err != ErrInvalidCreds {
		t.Fatalf("expected ErrInvalidCreds, got %v", err)
	}

	// Correct password must succeed
	authed, err := AuthenticateUser("alice", "secret123")
	if err != nil || authed.ID != user.ID {
		t.Fatalf("AuthenticateUser failed: %v", err)
	}

	// Auth session 7-day persistence
	sess, err := CreateAuthSession(user.ID, SessionTTL)
	if err != nil {
		t.Fatalf("CreateAuthSession failed: %v", err)
	}
	u2, s2, err := ValidateAuthSession(sess.Token)
	if err != nil || u2.ID != user.ID || s2.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("ValidateAuthSession failed: %v", err)
	}

	// Default workspace exists
	workspaces, err := ListWorkspaces(user.ID)
	if err != nil || len(workspaces) != 1 || workspaces[0].Name != "Default" || !workspaces[0].IsDefault {
		t.Fatalf("expected 1 Default workspace, got %+v (err=%v)", workspaces, err)
	}

	// Save workspace state (sessions + tasks + fps + token/network usage)
	wsID := workspaces[0].ID
	err = SaveWorkspaceState(user.ID, wsID,
		[]WorkspaceSession{{ID: "s1", IP: "127.0.0.1", Name: "Session: 127.0.0.1", Selected: true}},
		[]WorkspaceTask{{ID: "t1", SessionID: "s1", SessionIP: "127.0.0.1", Status: "in-progress", Message: "do stuff", SequenceNumber: 1}},
		15, 4200, 1048576,
	)
	if err != nil {
		t.Fatalf("SaveWorkspaceState failed: %v", err)
	}
	if err := SetActiveWorkspace(user.ID, wsID); err != nil {
		t.Fatalf("SetActiveWorkspace failed: %v", err)
	}

	workspaces, err = ListWorkspaces(user.ID)
	if err != nil || len(workspaces[0].Sessions) != 1 || len(workspaces[0].Tasks) != 1 || workspaces[0].FPS != 15 || workspaces[0].TokenUsage != 4200 || workspaces[0].NetworkUsage != 1048576 {
		t.Fatalf("expected saved sessions, tasks, fps, and usage, got %+v", workspaces[0])
	}

	// Create and delete second workspace
	ws2, err := CreateWorkspace(user.ID, "Prod")
	if err != nil {
		t.Fatalf("CreateWorkspace failed: %v", err)
	}
	if err := DeleteWorkspace(user.ID, wsID); err != ErrCannotDeleteLast {
		t.Fatalf("expected ErrCannotDeleteLast for Default workspace, got %v", err)
	}
	if err := DeleteWorkspace(user.ID, ws2.ID); err != nil {
		t.Fatalf("DeleteWorkspace failed: %v", err)
	}

	// Inventory CRUD
	srv, err := CreateInventoryServer(user.ID, "Dev Box", "10.0.0.5", "Ubuntu X11")
	if err != nil {
		t.Fatalf("CreateInventoryServer failed: %v", err)
	}
	list, err := ListInventory(user.ID)
	if err != nil || len(list) != 1 || list[0].IP != "10.0.0.5" {
		t.Fatalf("ListInventory unexpected: %+v", list)
	}
	if err := DeleteInventoryServer(user.ID, srv.ID); err != nil {
		t.Fatalf("DeleteInventoryServer failed: %v", err)
	}

	// Logout
	if err := DeleteAuthSession(sess.Token); err != nil {
		t.Fatalf("DeleteAuthSession failed: %v", err)
	}
	if _, _, err := ValidateAuthSession(sess.Token); err == nil {
		t.Fatalf("expected invalidated session to fail")
	}
}
