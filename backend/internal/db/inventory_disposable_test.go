package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDisposableInventory(t *testing.T) {
	if err := InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	user, err := RegisterUser("alice", "secret123")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}
	other, err := RegisterUser("bob", "secret123")
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	plain, err := CreateInventoryServer(user.ID, "host", "10.0.0.1", "")
	if err != nil {
		t.Fatalf("CreateInventoryServer failed: %v", err)
	}
	if plain.IsDisposable || plain.ContainerID != "" {
		t.Fatalf("regular server must not be disposable: %+v", plain)
	}

	disp, err := CreateDisposableInventoryServer(user.ID, "disposable-abc", "172.18.0.5", "temp", "container-1")
	if err != nil {
		t.Fatalf("CreateDisposableInventoryServer failed: %v", err)
	}
	if !disp.IsDisposable || disp.ContainerID != "container-1" || disp.IP != "172.18.0.5" {
		t.Fatalf("unexpected disposable row: %+v", disp)
	}

	got, err := GetInventoryServer(user.ID, disp.ID)
	if err != nil {
		t.Fatalf("GetInventoryServer failed: %v", err)
	}
	if !got.IsDisposable || got.ContainerID != "container-1" {
		t.Fatalf("disposable flags not persisted: %+v", got)
	}
	if _, err := GetInventoryServer(other.ID, disp.ID); err != ErrNotFound {
		t.Fatalf("another user must not see the row, got %v", err)
	}

	all, err := ListInventory(user.ID)
	if err != nil {
		t.Fatalf("ListInventory failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(all))
	}
	for _, s := range all {
		if s.ID == disp.ID && !s.IsDisposable {
			t.Fatalf("list must report the row as disposable: %+v", s)
		}
		if s.ID == plain.ID && s.IsDisposable {
			t.Fatalf("list must not report a regular server as disposable: %+v", s)
		}
	}

	if err := DeleteInventoryServer(user.ID, disp.ID); err != nil {
		t.Fatalf("DeleteInventoryServer failed: %v", err)
	}
	if _, err := GetInventoryServer(user.ID, disp.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// A database created before the disposable columns existed must be upgraded.
func TestInventoryMigrationFromOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	oldSchema := `
	CREATE TABLE users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL COLLATE NOCASE,
		password_hash TEXT NOT NULL,
		created_at INTEGER NOT NULL
	);
	CREATE TABLE inventory_servers (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		ip TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	INSERT INTO users (id, username, password_hash, created_at) VALUES ('u1', 'alice', 'x', 1);
	INSERT INTO inventory_servers (id, user_id, name, ip, description, created_at, updated_at)
		VALUES ('srv1', 'u1', 'host', '10.0.0.1', '', 1, 1);
	`
	if _, err := conn.Exec(oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	_ = conn.Close()

	if err := InitDB(path); err != nil {
		t.Fatalf("InitDB on old database failed: %v", err)
	}
	servers, err := ListInventory("u1")
	if err != nil {
		t.Fatalf("ListInventory failed: %v", err)
	}
	if len(servers) != 1 || servers[0].ID != "srv1" || servers[0].IsDisposable {
		t.Fatalf("old row not migrated correctly: %+v", servers)
	}
}
