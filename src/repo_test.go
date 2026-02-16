package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyRepoGitHubHTTPS(t *testing.T) {
	tests := []struct {
		input    string
		wantKind RepoKind
		wantName string
	}{
		{"https://github.com/user/repo", RepoGitURL, "repo"},
		{"https://github.com/user/repo.git", RepoGitURL, "repo"},
		{"https://github.com/org/my-project/", RepoGitURL, "my-project"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, name, kind := classifyRepo(tt.input)
			if kind != tt.wantKind {
				t.Errorf("classifyRepo(%q) kind = %d, want %d", tt.input, kind, tt.wantKind)
			}
			if name != tt.wantName {
				t.Errorf("classifyRepo(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
		})
	}
}

func TestClassifyRepoGitSSH(t *testing.T) {
	url, name, kind := classifyRepo("git@github.com:user/repo.git")
	if kind != RepoGitURL {
		t.Errorf("expected RepoGitURL, got %d", kind)
	}
	if name != "repo" {
		t.Errorf("expected name repo, got %s", name)
	}
	if url != "git@github.com:user/repo.git" {
		t.Errorf("expected URL preserved, got %s", url)
	}
}

func TestClassifyRepoShorthand(t *testing.T) {
	url, name, kind := classifyRepo("user/repo")
	if kind != RepoGitURL {
		t.Errorf("expected RepoGitURL, got %d", kind)
	}
	if name != "repo" {
		t.Errorf("expected name repo, got %s", name)
	}
	if url != "https://github.com/user/repo.git" {
		t.Errorf("expected expanded URL, got %s", url)
	}
}

func TestClassifyRepoLocal(t *testing.T) {
	_, _, kind := classifyRepo("/some/local/path")
	if kind != RepoLocal {
		t.Errorf("expected RepoLocal, got %d", kind)
	}
}

func TestCompletePath(t *testing.T) {
	dir := t.TempDir()
	// Create some subdirs
	os.Mkdir(filepath.Join(dir, "alpha"), 0755)
	os.Mkdir(filepath.Join(dir, "beta"), 0755)
	os.Mkdir(filepath.Join(dir, ".hidden"), 0755)
	// Create a file (should not appear)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644)

	results := CompletePath(dir+"/", 10)
	if len(results) != 2 {
		t.Errorf("expected 2 visible dirs, got %d: %v", len(results), results)
	}

	// With prefix filter
	results = CompletePath(dir+"/al", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 match for 'al', got %d: %v", len(results), results)
	}

	// Dotfiles shown when prefix starts with .
	results = CompletePath(dir+"/.", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 dotfile match, got %d: %v", len(results), results)
	}
}

func TestIsGitRepo(t *testing.T) {
	// Non-git dir
	dir := t.TempDir()
	if IsGitRepo(dir) {
		t.Error("expected false for non-git dir")
	}

	// Create .git dir to simulate git repo
	os.Mkdir(filepath.Join(dir, ".git"), 0755)
	if !IsGitRepo(dir) {
		t.Error("expected true for dir with .git")
	}
}

func TestCompletePathEmpty(t *testing.T) {
	// Should not panic on empty input
	results := CompletePath("", 5)
	// Returns whatever is in ./ — just ensure no panic
	_ = results
}
