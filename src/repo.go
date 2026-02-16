package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Matches github.com URLs: https://github.com/user/repo or https://github.com/user/repo.git
	githubHTTPS = regexp.MustCompile(`^https?://github\.com/([^/]+/[^/]+?)(?:\.git)?/?$`)
	// Matches git SSH URIs: git@github.com:user/repo.git
	gitSSH = regexp.MustCompile(`^git@([^:]+):([^/]+/[^/]+?)(?:\.git)?$`)
	// Matches shorthand: user/repo (GitHub assumed)
	shorthand = regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`)
)

// RepoKind describes what kind of repo reference was given.
type RepoKind int

const (
	RepoLocal RepoKind = iota
	RepoGitURL
)

// ResolveRepo takes user input and returns a local path to the repo.
// If the input is a URL or git URI, it clones the repo into the workspace's repos directory.
// If it's a local path, it expands ~ and validates it exists.
func ResolveRepo(input string, workspaceDir string) (localPath string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty repo path")
	}

	cloneURL, repoName, kind := classifyRepo(input)

	switch kind {
	case RepoLocal:
		// Expand ~ to home dir
		if strings.HasPrefix(input, "~/") {
			home, _ := os.UserHomeDir()
			input = filepath.Join(home, input[2:])
		}
		// Resolve to absolute path
		abs, err := filepath.Abs(input)
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}
		// Check it exists
		info, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("path not found: %s", abs)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("not a directory: %s", abs)
		}
		return abs, nil

	case RepoGitURL:
		// Clone into workspace repos dir
		reposDir := filepath.Join(workspaceDir, "repos")
		if err := os.MkdirAll(reposDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create repos dir: %w", err)
		}

		dest := filepath.Join(reposDir, repoName)

		// If already cloned, return existing path
		if _, err := os.Stat(dest); err == nil {
			return dest, nil
		}

		fmt.Fprintf(os.Stderr, "[xue] Cloning %s...\n", cloneURL)

		cmd := exec.Command("git", "clone", cloneURL, dest)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone failed: %w", err)
		}

		return dest, nil
	}

	return "", fmt.Errorf("could not resolve repo: %s", input)
}

// classifyRepo determines if the input is a local path, git URL, or shorthand.
func classifyRepo(input string) (cloneURL, repoName string, kind RepoKind) {
	// GitHub HTTPS URL
	if m := githubHTTPS.FindStringSubmatch(input); m != nil {
		nameWithOwner := m[1]
		parts := strings.Split(nameWithOwner, "/")
		return input, parts[len(parts)-1], RepoGitURL
	}

	// Git SSH URI
	if m := gitSSH.FindStringSubmatch(input); m != nil {
		nameWithOwner := m[2]
		parts := strings.Split(nameWithOwner, "/")
		return input, parts[len(parts)-1], RepoGitURL
	}

	// Shorthand: user/repo → https://github.com/user/repo.git
	if shorthand.MatchString(input) {
		parts := strings.Split(input, "/")
		return "https://github.com/" + input + ".git", parts[1], RepoGitURL
	}

	// Assume local path
	return input, filepath.Base(input), RepoLocal
}

// CompletePath returns filesystem path completions for the given partial input.
// Returns up to maxResults matching directories.
func CompletePath(partial string, maxResults int) []string {
	if partial == "" {
		partial = "./"
	}

	// Expand ~
	expanded := partial
	if strings.HasPrefix(expanded, "~/") {
		home, _ := os.UserHomeDir()
		expanded = filepath.Join(home, expanded[2:])
	}

	// If partial ends with /, list contents of that directory
	// Otherwise, list contents of parent directory filtered by prefix
	dir := expanded
	prefix := ""
	if !strings.HasSuffix(expanded, "/") {
		dir = filepath.Dir(expanded)
		prefix = filepath.Base(expanded)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var results []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue // hide dotfiles unless user typed a dot
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			continue
		}

		// Build the display path (using the original ~ prefix if applicable)
		var fullPath string
		if strings.HasPrefix(partial, "~/") {
			relToHome := strings.TrimPrefix(dir, "")
			if home, _ := os.UserHomeDir(); strings.HasPrefix(dir, home) {
				relToHome = "~" + strings.TrimPrefix(dir, home)
			}
			fullPath = filepath.Join(relToHome, name)
		} else {
			fullPath = filepath.Join(dir, name)
		}
		// Append / to signal it's a directory
		fullPath += "/"

		results = append(results, fullPath)
		if len(results) >= maxResults {
			break
		}
	}

	return results
}
