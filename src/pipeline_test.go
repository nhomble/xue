package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPipelineValid(t *testing.T) {
	yaml := `
name: test-pipeline
steps:
  - name: step1
    command: echo hello
  - name: step2
    command: echo world
`
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := loadPipeline(path)
	if err != nil {
		t.Fatalf("loadPipeline failed: %v", err)
	}
	if cfg.Name != "test-pipeline" {
		t.Errorf("expected name test-pipeline, got %s", cfg.Name)
	}
	if len(cfg.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(cfg.Steps))
	}
}

func TestLoadPipelineMissingName(t *testing.T) {
	yaml := `
steps:
  - command: echo hello
`
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	_, err := loadPipeline(path)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestLoadPipelineNoSteps(t *testing.T) {
	yaml := `
name: empty
steps: []
`
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	_, err := loadPipeline(path)
	if err == nil {
		t.Error("expected error for no steps")
	}
}

func TestLoadPipelineStepMissingCommand(t *testing.T) {
	yaml := `
name: bad
steps:
  - name: oops
`
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	_, err := loadPipeline(path)
	if err == nil {
		t.Error("expected error for step missing command")
	}
}

func TestLoadPipelineAutoName(t *testing.T) {
	yaml := `
name: autoname
steps:
  - command: echo hello
`
	path := filepath.Join(t.TempDir(), "pipeline.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := loadPipeline(path)
	if err != nil {
		t.Fatalf("loadPipeline failed: %v", err)
	}
	if cfg.Steps[0].Name != "step-1" {
		t.Errorf("expected auto name step-1, got %s", cfg.Steps[0].Name)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("short string should not be truncated")
	}
	if truncate("hello world this is long", 10) != "hello worl..." {
		t.Errorf("long string should be truncated, got %q", truncate("hello world this is long", 10))
	}
	// Newlines should be replaced
	if got := truncate("line1\nline2", 20); got != "line1 line2" {
		t.Errorf("newlines should be replaced, got %q", got)
	}
}
