package main

import "testing"

func TestWordWrap(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		contains string
	}{
		{"short line", "hello", 80, "hello"},
		{"wraps at space", "hello world", 8, "hello\nworld"},
		{"hard break", "abcdefghij", 5, "abcde\nfghij"},
		{"empty", "", 80, ""},
		{"preserves newlines", "a\nb", 80, "a\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.input, tt.width)
			if tt.contains != "" && !containsSubstring(got, tt.contains) {
				t.Errorf("wordWrap(%q, %d) = %q, want to contain %q", tt.input, tt.width, got, tt.contains)
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

func TestTruncateStr(t *testing.T) {
	if truncateStr("hello", 10) != "hello" {
		t.Error("short string should not change")
	}
	got := truncateStr("hello world", 8)
	if got != "hello..." {
		t.Errorf("expected 'hello...', got %q", got)
	}
	// Very small maxLen
	if truncateStr("abcdef", 4) != "a..." {
		t.Errorf("expected 'a...', got %q", truncateStr("abcdef", 4))
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"/absolute/path", true},
		{"~/home/path", true},
		{"./relative", true},
		{"../parent", true},
		{"https://github.com/user/repo", false},
		{"git@github.com:user/repo", false},
		{"somedir", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isLocalPath(tt.input)
			if got != tt.want {
				t.Errorf("isLocalPath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
