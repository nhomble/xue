package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ThreadStatus int

const (
	ThreadRunning ThreadStatus = iota
	ThreadStopped
	ThreadExited
)

func (s ThreadStatus) String() string {
	switch s {
	case ThreadRunning:
		return "checking..."
	case ThreadStopped:
		return "stopped"
	case ThreadExited:
		return "done"
	default:
		return "unknown"
	}
}

// Thread represents a headless claude question.
type Thread struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspace_id"`
	ParentID    string       `json:"parent_id,omitempty"` // for follow-up chains
	Question    string       `json:"question"`
	Repos       []string     `json:"repos"`
	Status      ThreadStatus `json:"status"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  time.Time    `json:"finished_at,omitempty"`
	ExitCode    int          `json:"exit_code,omitempty"`
	AnswerText  string       `json:"answer,omitempty"` // persisted answer

	cmd    *exec.Cmd    `json:"-"`
	mu     sync.Mutex   `json:"-"`
	output bytes.Buffer `json:"-"`
	doneCh chan struct{} `json:"-"`
}

// ThreadManager manages threads and persists them.
type ThreadManager struct {
	mu      sync.Mutex
	threads []*Thread
	store   *WorkspaceStore
}

func NewThreadManager(store *WorkspaceStore) *ThreadManager {
	return &ThreadManager{
		store: store,
	}
}

// LoadThreads loads persisted threads for a workspace from disk.
func (tm *ThreadManager) LoadThreads(workspaceID string) {
	dir := tm.store.ThreadDir(workspaceID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var t Thread
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}

		// Skip if already loaded
		found := false
		for _, existing := range tm.threads {
			if existing.ID == t.ID {
				found = true
				break
			}
		}
		if found {
			continue
		}

		// Restore the output buffer from persisted answer
		t.output.WriteString(t.AnswerText)
		t.doneCh = make(chan struct{})
		close(t.doneCh) // already finished

		tm.threads = append(tm.threads, &t)
	}
}

// LoadAllThreads loads threads for all workspaces.
func (tm *ThreadManager) LoadAllThreads() {
	workspaces, err := tm.store.List()
	if err != nil {
		return
	}
	for _, ws := range workspaces {
		tm.LoadThreads(ws.ID)
	}
}

// persist saves a completed thread to disk.
func (tm *ThreadManager) persist(t *Thread) {
	dir := tm.store.ThreadDir(t.WorkspaceID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	// Copy answer to the persisted field
	t.mu.Lock()
	t.AnswerText = t.output.String()
	t.mu.Unlock()

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return
	}

	path := filepath.Join(dir, t.ID+".json")
	os.WriteFile(path, data, 0644)
}

func (tm *ThreadManager) Spawn(question, workspaceID string) (*Thread, error) {
	return tm.SpawnWithParent(question, workspaceID, "")
}

func (tm *ThreadManager) SpawnWithParent(question, workspaceID, parentID string) (*Thread, error) {
	if question == "" {
		return nil, fmt.Errorf("empty question")
	}

	ws, err := tm.store.Get(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// Build conversation context from parent chain
	contextPrompt := tm.buildConversationContext(parentID)

	// The actual prompt sent to claude: context + new question
	fullPrompt := question
	if contextPrompt != "" {
		fullPrompt = contextPrompt + "\n\nFollow-up question: " + question
	}

	args := []string{"-p", fullPrompt}
	if ws.Settings.Model != "" {
		args = append(args, "--model", ws.Settings.Model)
	}
	if ws.Settings.MaxTokens > 0 {
		args = append(args, "--max-tokens", fmt.Sprintf("%d", ws.Settings.MaxTokens))
	}
	if ws.Settings.SystemPrompt != "" {
		args = append(args, "--system-prompt", ws.Settings.SystemPrompt)
	}
	for _, repo := range ws.Repos {
		args = append(args, "--add-dir", repo)
	}

	id := generateID()
	cmd := exec.Command("claude", args...)

	// Unset CLAUDECODE so claude doesn't refuse to run
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	if len(ws.Repos) > 0 {
		cmd.Dir = ws.Repos[0]
	}

	t := &Thread{
		ID:          id,
		WorkspaceID: workspaceID,
		ParentID:    parentID,
		Question:    question,
		Repos:       ws.Repos,
		Status:      ThreadRunning,
		StartedAt:   time.Now(),
		cmd:         cmd,
		doneCh:      make(chan struct{}),
	}

	cmd.Stdout = &threadWriter{thread: t}
	cmd.Stderr = &threadWriter{thread: t}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start claude: %w", err)
	}

	go func() {
		err := cmd.Wait()
		t.mu.Lock()
		t.Status = ThreadExited
		t.FinishedAt = time.Now()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				t.ExitCode = exitErr.ExitCode()
			} else {
				t.output.WriteString("\n[error: " + err.Error() + "]\n")
			}
		}
		t.mu.Unlock()
		close(t.doneCh)

		// Persist to disk
		tm.persist(t)
	}()

	tm.mu.Lock()
	tm.threads = append(tm.threads, t)
	tm.mu.Unlock()

	return t, nil
}

type threadWriter struct {
	thread *Thread
}

func (w *threadWriter) Write(p []byte) (int, error) {
	w.thread.mu.Lock()
	defer w.thread.mu.Unlock()
	return w.thread.output.Write(p)
}

func (tm *ThreadManager) Threads() []*Thread {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	result := make([]*Thread, len(tm.threads))
	copy(result, tm.threads)
	return result
}

// ThreadsForWorkspace returns threads belonging to a specific workspace.
func (tm *ThreadManager) ThreadsForWorkspace(workspaceID string) []*Thread {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var result []*Thread
	for _, t := range tm.threads {
		if t.WorkspaceID == workspaceID {
			result = append(result, t)
		}
	}
	return result
}

// RootThreadsForWorkspace returns only top-level threads (no parent) for a workspace,
// sorted by most recent activity (latest timestamp in the chain).
func (tm *ThreadManager) RootThreadsForWorkspace(workspaceID string) []*Thread {
	all := tm.ThreadsForWorkspace(workspaceID)
	var roots []*Thread
	for _, t := range all {
		if t.ParentID == "" {
			roots = append(roots, t)
		}
	}
	// Sort by latest activity in chain (most recent first)
	sort.Slice(roots, func(i, j int) bool {
		ti := tm.latestActivity(roots[i])
		tj := tm.latestActivity(roots[j])
		return ti.After(tj)
	})
	return roots
}

// GetChain returns the full conversation chain starting from a root thread,
// in chronological order (root first, then each follow-up).
func (tm *ThreadManager) GetChain(rootID string) []*Thread {
	// Build a map of parentID → children
	tm.mu.Lock()
	children := make(map[string][]*Thread)
	for _, t := range tm.threads {
		if t.ParentID != "" {
			children[t.ParentID] = append(children[t.ParentID], t)
		}
	}
	tm.mu.Unlock()

	var chain []*Thread
	current := tm.Get(rootID)
	for current != nil {
		chain = append(chain, current)
		kids := children[current.ID]
		if len(kids) == 0 {
			break
		}
		// Follow the most recent child (in case of branching, pick latest)
		latest := kids[0]
		for _, k := range kids[1:] {
			if k.StartedAt.After(latest.StartedAt) {
				latest = k
			}
		}
		current = latest
	}
	return chain
}

// FollowUpCount returns how many follow-ups exist in a thread's chain (excluding the root).
func (tm *ThreadManager) FollowUpCount(rootID string) int {
	chain := tm.GetChain(rootID)
	if len(chain) <= 1 {
		return 0
	}
	return len(chain) - 1
}

// latestActivity returns the most recent timestamp in a thread's chain.
func (tm *ThreadManager) latestActivity(root *Thread) time.Time {
	chain := tm.GetChain(root.ID)
	latest := root.StartedAt
	for _, t := range chain {
		if !t.FinishedAt.IsZero() && t.FinishedAt.After(latest) {
			latest = t.FinishedAt
		}
		if t.StartedAt.After(latest) {
			latest = t.StartedAt
		}
	}
	return latest
}

func (tm *ThreadManager) Get(id string) *Thread {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for _, t := range tm.threads {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (tm *ThreadManager) Stop(id string) {
	t := tm.Get(id)
	if t == nil || t.Status != ThreadRunning {
		return
	}
	t.cmd.Process.Signal(syscall.SIGTERM)
	go func() {
		time.Sleep(3 * time.Second)
		t.mu.Lock()
		if t.Status == ThreadRunning {
			t.cmd.Process.Kill()
		}
		t.mu.Unlock()
	}()
}

func (tm *ThreadManager) Remove(id string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	for i, t := range tm.threads {
		if t.ID == id {
			if t.Status == ThreadRunning && t.cmd != nil && t.cmd.Process != nil {
				t.cmd.Process.Signal(syscall.SIGTERM)
			}
			// Delete persisted file
			if t.WorkspaceID != "" {
				path := filepath.Join(tm.store.ThreadDir(t.WorkspaceID), t.ID+".json")
				os.Remove(path)
			}
			tm.threads = append(tm.threads[:i], tm.threads[i+1:]...)
			return
		}
	}
}

func (tm *ThreadManager) StopAll() {
	tm.mu.Lock()
	threads := make([]*Thread, len(tm.threads))
	copy(threads, tm.threads)
	tm.mu.Unlock()

	for _, t := range threads {
		if t.Status == ThreadRunning && t.cmd != nil && t.cmd.Process != nil {
			t.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
}

// Answer returns the thread's output as a string.
func (t *Thread) Answer() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.output.String()
	if s != "" {
		return s
	}
	return t.AnswerText
}

// Preview returns a short preview of the answer.
func (t *Thread) Preview(maxLen int) string {
	answer := t.Answer()
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return ""
	}
	lines := strings.SplitN(answer, "\n", 2)
	preview := strings.TrimSpace(lines[0])
	if len(preview) > maxLen {
		preview = preview[:maxLen] + "..."
	}
	return preview
}

// Duration returns how long this thread took (or has been running).
func (t *Thread) Duration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.FinishedAt.IsZero() {
		return t.FinishedAt.Sub(t.StartedAt).Truncate(time.Second)
	}
	return time.Since(t.StartedAt).Truncate(time.Second)
}

// buildConversationContext walks the parent chain and builds a conversation
// history string that gives the model context for follow-up questions.
func (tm *ThreadManager) buildConversationContext(parentID string) string {
	if parentID == "" {
		return ""
	}

	// Walk the chain collecting Q&A pairs (child → parent order)
	var chain []struct{ q, a string }
	current := parentID
	for current != "" {
		t := tm.Get(current)
		if t == nil {
			break
		}
		answer := t.Answer()
		// Truncate very long answers to keep prompt reasonable
		if len(answer) > 8000 {
			answer = answer[:8000] + "\n[... truncated]"
		}
		chain = append(chain, struct{ q, a string }{t.Question, answer})
		current = t.ParentID
	}

	if len(chain) == 0 {
		return ""
	}

	// Reverse to chronological order (oldest first)
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	var buf strings.Builder
	buf.WriteString("Previous conversation:\n")
	for _, qa := range chain {
		buf.WriteString("\nQ: " + qa.q + "\n")
		buf.WriteString("A: " + qa.a + "\n")
	}
	return buf.String()
}

// ChainDepth returns how many parent threads this thread has.
func (tm *ThreadManager) ChainDepth(t *Thread) int {
	depth := 0
	current := t.ParentID
	for current != "" {
		depth++
		parent := tm.Get(current)
		if parent == nil {
			break
		}
		current = parent.ParentID
	}
	return depth
}
