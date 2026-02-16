package main

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *WorkspaceStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "workspaces")
	os.MkdirAll(dir, 0755)
	return &WorkspaceStore{dir: dir}
}

func TestWorkspaceCreateAndGet(t *testing.T) {
	store := newTestStore(t)

	ws, err := store.Create("test-ws", []string{"/tmp/repo1"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if ws.Name != "test-ws" {
		t.Errorf("expected name test-ws, got %s", ws.Name)
	}
	if len(ws.Repos) != 1 || ws.Repos[0] != "/tmp/repo1" {
		t.Errorf("unexpected repos: %v", ws.Repos)
	}

	got, err := store.Get(ws.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name != ws.Name {
		t.Errorf("Get returned name %s, want %s", got.Name, ws.Name)
	}
}

func TestWorkspaceList(t *testing.T) {
	store := newTestStore(t)

	store.Create("first", nil)
	store.Create("second", nil)

	list, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(list))
	}
}

func TestWorkspaceDelete(t *testing.T) {
	store := newTestStore(t)

	ws, _ := store.Create("delete-me", nil)
	if err := store.Delete(ws.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Get(ws.ID)
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestWorkspaceAddRepo(t *testing.T) {
	store := newTestStore(t)
	ws, _ := store.Create("ws", nil)

	// Create a temp dir to use as a local repo
	repoDir := t.TempDir()
	path, err := store.AddRepo(ws.ID, repoDir)
	if err != nil {
		t.Fatalf("AddRepo failed: %v", err)
	}
	if path != repoDir {
		t.Errorf("expected path %s, got %s", repoDir, path)
	}

	// Verify it was persisted
	got, _ := store.Get(ws.ID)
	if len(got.Repos) != 1 {
		t.Errorf("expected 1 repo, got %d", len(got.Repos))
	}

	// Adding same repo again should be idempotent
	_, err = store.AddRepo(ws.ID, repoDir)
	if err != nil {
		t.Fatalf("AddRepo duplicate failed: %v", err)
	}
	got, _ = store.Get(ws.ID)
	if len(got.Repos) != 1 {
		t.Errorf("expected still 1 repo after duplicate add, got %d", len(got.Repos))
	}
}

func TestWorkspaceUpdateSettings(t *testing.T) {
	store := newTestStore(t)
	ws, _ := store.Create("ws", nil)

	settings := WorkspaceSettings{Model: "opus", MaxTokens: 4096}
	if err := store.UpdateSettings(ws.ID, settings); err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	got, _ := store.Get(ws.ID)
	if got.Settings.Model != "opus" {
		t.Errorf("expected model opus, got %s", got.Settings.Model)
	}
	if got.Settings.MaxTokens != 4096 {
		t.Errorf("expected max_tokens 4096, got %d", got.Settings.MaxTokens)
	}
}

func TestFormatRepoList(t *testing.T) {
	if FormatRepoList(nil) != "(no repos)" {
		t.Error("expected (no repos) for nil")
	}
	if FormatRepoList([]string{"/a/b/myrepo"}) != "myrepo" {
		t.Error("expected base name for single repo")
	}
	got := FormatRepoList([]string{"/a/first", "/b/second", "/c/third"})
	if got != "first +2 more" {
		t.Errorf("expected 'first +2 more', got %q", got)
	}
}

func TestGenerateID(t *testing.T) {
	id := generateID()
	if len(id) != 16 {
		t.Errorf("expected 16-char hex ID, got %d chars: %s", len(id), id)
	}
	// Should be unique
	id2 := generateID()
	if id == id2 {
		t.Error("two generated IDs should not be equal")
	}
}

func TestModelDisplayName(t *testing.T) {
	if ModelDisplayName("") != "default" {
		t.Error("empty model should return 'default'")
	}
	if ModelDisplayName("opus") != "opus" {
		t.Error("non-empty model should return itself")
	}
}
