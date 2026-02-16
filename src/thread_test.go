package main

import (
	"strings"
	"testing"
	"time"
)

func newTestThreadManager() *ThreadManager {
	return &ThreadManager{}
}

func makeThread(id, wsID, parentID, question, answer string, status ThreadStatus) *Thread {
	t := &Thread{
		ID:          id,
		WorkspaceID: wsID,
		ParentID:    parentID,
		Question:    question,
		Status:      status,
		StartedAt:   time.Now().Add(-10 * time.Second),
		doneCh:      make(chan struct{}),
	}
	if status == ThreadExited {
		t.FinishedAt = time.Now()
		close(t.doneCh)
	}
	t.output.WriteString(answer)
	return t
}

func TestThreadPreview(t *testing.T) {
	th := makeThread("1", "ws1", "", "question", "First line here\nSecond line", ThreadExited)

	preview := th.Preview(10)
	if preview != "First line..." {
		t.Errorf("expected truncated preview, got %q", preview)
	}

	preview = th.Preview(100)
	if preview != "First line here" {
		t.Errorf("expected first line, got %q", preview)
	}
}

func TestThreadPreviewEmpty(t *testing.T) {
	th := makeThread("1", "ws1", "", "question", "", ThreadRunning)
	if th.Preview(50) != "" {
		t.Error("expected empty preview for no answer")
	}
}

func TestThreadAnswer(t *testing.T) {
	th := makeThread("1", "ws1", "", "q", "buffer content", ThreadExited)
	if th.Answer() != "buffer content" {
		t.Errorf("expected buffer content, got %q", th.Answer())
	}

	// When buffer is empty, falls back to AnswerText
	th2 := &Thread{AnswerText: "persisted answer"}
	if th2.Answer() != "persisted answer" {
		t.Errorf("expected persisted answer, got %q", th2.Answer())
	}
}

func TestThreadDuration(t *testing.T) {
	now := time.Now()
	th := &Thread{
		StartedAt:  now.Add(-5 * time.Second),
		FinishedAt: now,
	}
	d := th.Duration()
	if d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}
}

func TestThreadDurationRunning(t *testing.T) {
	th := &Thread{
		StartedAt: time.Now().Add(-2 * time.Second),
	}
	d := th.Duration()
	if d < 1*time.Second || d > 5*time.Second {
		t.Errorf("expected ~2s for running thread, got %v", d)
	}
}

func TestGetChain(t *testing.T) {
	tm := newTestThreadManager()
	root := makeThread("root", "ws1", "", "first question", "answer1", ThreadExited)
	child := makeThread("child", "ws1", "root", "follow-up", "answer2", ThreadExited)
	grandchild := makeThread("grand", "ws1", "child", "another follow-up", "answer3", ThreadExited)

	tm.threads = []*Thread{root, child, grandchild}

	chain := tm.GetChain("root")
	if len(chain) != 3 {
		t.Fatalf("expected chain of 3, got %d", len(chain))
	}
	if chain[0].ID != "root" || chain[1].ID != "child" || chain[2].ID != "grand" {
		t.Errorf("unexpected chain order: %s, %s, %s", chain[0].ID, chain[1].ID, chain[2].ID)
	}
}

func TestRootThreadsForWorkspace(t *testing.T) {
	tm := newTestThreadManager()
	root1 := makeThread("r1", "ws1", "", "q1", "a1", ThreadExited)
	root2 := makeThread("r2", "ws1", "", "q2", "a2", ThreadExited)
	child := makeThread("c1", "ws1", "r1", "follow-up", "a3", ThreadExited)
	otherWs := makeThread("r3", "ws2", "", "q3", "a4", ThreadExited)

	tm.threads = []*Thread{root1, root2, child, otherWs}

	roots := tm.RootThreadsForWorkspace("ws1")
	if len(roots) != 2 {
		t.Errorf("expected 2 root threads for ws1, got %d", len(roots))
	}
	for _, r := range roots {
		if r.ParentID != "" {
			t.Errorf("root thread %s has parent %s", r.ID, r.ParentID)
		}
	}
}

func TestBuildConversationContext(t *testing.T) {
	tm := newTestThreadManager()
	root := makeThread("root", "ws1", "", "what is go?", "Go is a programming language.", ThreadExited)
	child := makeThread("child", "ws1", "root", "who created it?", "Rob Pike et al.", ThreadExited)

	tm.threads = []*Thread{root, child}

	// buildConversationContext("child") means child is the parent of the NEW thread.
	// So it includes both root and child Q&A as context.
	ctx := tm.buildConversationContext("child")
	if !strings.Contains(ctx, "what is go?") {
		t.Error("context should contain root question")
	}
	if !strings.Contains(ctx, "Go is a programming language") {
		t.Error("context should contain root answer")
	}
	if !strings.Contains(ctx, "who created it?") {
		t.Error("context should contain child question")
	}
	if !strings.Contains(ctx, "Rob Pike et al.") {
		t.Error("context should contain child answer")
	}

	// Context from just the root
	ctx2 := tm.buildConversationContext("root")
	if !strings.Contains(ctx2, "what is go?") {
		t.Error("context from root should contain root question")
	}
	if strings.Contains(ctx2, "who created it?") {
		t.Error("context from root should not contain child question")
	}
}

func TestBuildConversationContextEmpty(t *testing.T) {
	tm := newTestThreadManager()
	if tm.buildConversationContext("") != "" {
		t.Error("empty parentID should return empty context")
	}
}

func TestChainDepth(t *testing.T) {
	tm := newTestThreadManager()
	root := makeThread("root", "ws1", "", "q", "a", ThreadExited)
	child := makeThread("child", "ws1", "root", "q2", "a2", ThreadExited)
	grand := makeThread("grand", "ws1", "child", "q3", "a3", ThreadExited)
	tm.threads = []*Thread{root, child, grand}

	if tm.ChainDepth(root) != 0 {
		t.Errorf("root depth should be 0, got %d", tm.ChainDepth(root))
	}
	if tm.ChainDepth(child) != 1 {
		t.Errorf("child depth should be 1, got %d", tm.ChainDepth(child))
	}
	if tm.ChainDepth(grand) != 2 {
		t.Errorf("grandchild depth should be 2, got %d", tm.ChainDepth(grand))
	}
}

func TestFollowUpCount(t *testing.T) {
	tm := newTestThreadManager()
	root := makeThread("root", "ws1", "", "q", "a", ThreadExited)
	child := makeThread("child", "ws1", "root", "q2", "a2", ThreadExited)
	tm.threads = []*Thread{root, child}

	if tm.FollowUpCount("root") != 1 {
		t.Errorf("expected 1 follow-up, got %d", tm.FollowUpCount("root"))
	}
	if tm.FollowUpCount("child") != 0 {
		t.Errorf("expected 0 follow-ups for leaf, got %d", tm.FollowUpCount("child"))
	}
}
