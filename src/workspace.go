package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// WorkspaceSettings holds per-workspace configuration.
type WorkspaceSettings struct {
	Model       string `json:"model,omitempty"`        // claude model to use (e.g. "sonnet", "opus")
	MaxTokens   int    `json:"max_tokens,omitempty"`   // max output tokens (0 = default)
	SystemPrompt string `json:"system_prompt,omitempty"` // custom system prompt prefix
}

// AvailableModels lists the models users can pick from.
var AvailableModels = []string{
	"",            // default (whatever claude CLI defaults to)
	"sonnet",
	"opus",
	"haiku",
}

// ModelDisplayName returns a human-readable name for a model value.
func ModelDisplayName(model string) string {
	if model == "" {
		return "default"
	}
	return model
}

type Workspace struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Repos        []string          `json:"repos"`
	Settings     WorkspaceSettings `json:"settings"`
	CreatedAt    time.Time         `json:"created_at"`
	LastAccessed time.Time         `json:"last_accessed"`
}

type WorkspaceStore struct {
	dir string
}

func NewWorkspaceStore() (*WorkspaceStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".xue", "workspaces")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &WorkspaceStore{dir: dir}, nil
}

func (s *WorkspaceStore) Create(name string) (*Workspace, error) {
	id := generateID()
	w := &Workspace{
		ID:           id,
		Name:         name,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
	}

	// Create workspace data directory for threads/logs
	dataDir := filepath.Join(s.dir, id, "threads")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	return w, s.save(w)
}

func (s *WorkspaceStore) List() ([]*Workspace, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var workspaces []*Workspace
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		w, err := s.load(e.Name())
		if err != nil {
			continue
		}
		workspaces = append(workspaces, w)
	}

	sort.Slice(workspaces, func(i, j int) bool {
		return workspaces[i].LastAccessed.After(workspaces[j].LastAccessed)
	})

	return workspaces, nil
}

func (s *WorkspaceStore) Get(id string) (*Workspace, error) {
	return s.load(id)
}

func (s *WorkspaceStore) Touch(id string) error {
	w, err := s.load(id)
	if err != nil {
		return err
	}
	w.LastAccessed = time.Now()
	return s.save(w)
}

func (s *WorkspaceStore) Delete(id string) error {
	return os.RemoveAll(filepath.Join(s.dir, id))
}

func (s *WorkspaceStore) AddRepo(id, input string) (string, error) {
	w, err := s.load(id)
	if err != nil {
		return "", err
	}

	wsDir := filepath.Join(s.dir, id)
	localPath, err := ResolveRepo(input, wsDir)
	if err != nil {
		return "", err
	}

	for _, r := range w.Repos {
		if r == localPath {
			return localPath, nil // already attached
		}
	}
	w.Repos = append(w.Repos, localPath)
	return localPath, s.save(w)
}

func (s *WorkspaceStore) UpdateSettings(id string, settings WorkspaceSettings) error {
	w, err := s.load(id)
	if err != nil {
		return err
	}
	w.Settings = settings
	return s.save(w)
}

func (s *WorkspaceStore) ThreadDir(workspaceID string) string {
	return filepath.Join(s.dir, workspaceID, "threads")
}

func (s *WorkspaceStore) save(w *Workspace) error {
	dir := filepath.Join(s.dir, w.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "workspace.json"), data, 0644)
}

func (s *WorkspaceStore) load(id string) (*Workspace, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id, "workspace.json"))
	if err != nil {
		return nil, err
	}
	var w Workspace
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ThreadLogDir returns the path for a specific thread's logs within a workspace.
func (s *WorkspaceStore) ThreadLogDir(workspaceID, threadID string) string {
	return filepath.Join(s.dir, workspaceID, "threads", threadID)
}

// FormatRepoList returns a short display string for a workspace's repos.
func FormatRepoList(repos []string) string {
	if len(repos) == 0 {
		return "(no repos)"
	}
	result := filepath.Base(repos[0])
	if len(repos) > 1 {
		result += fmt.Sprintf(" +%d more", len(repos)-1)
	}
	return result
}
