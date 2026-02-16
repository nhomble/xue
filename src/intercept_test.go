package main

import "testing"

func TestInterceptorSlashCommands(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Command
	}{
		{"end", "/end\r", CommandEnd},
		{"next", "/next\r", CommandNext},
		{"log", "/log\r", CommandLog},
		{"status", "/status\r", CommandStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var flushed []byte
			ic := NewInterceptor(func(b []byte) {
				flushed = append(flushed, b...)
			}, nil, "test")

			got := ic.Feed([]byte(tt.input))
			if got != tt.want {
				t.Errorf("Feed(%q) = %d, want %d", tt.input, got, tt.want)
			}
			if len(flushed) != 0 {
				t.Errorf("expected no flush for valid command, got %q", flushed)
			}
		})
	}
}

func TestInterceptorNonCommand(t *testing.T) {
	var flushed []byte
	ic := NewInterceptor(func(b []byte) {
		flushed = append(flushed, b...)
	}, nil, "test")

	got := ic.Feed([]byte("/foo\r"))
	if got != CommandNone {
		t.Errorf("expected CommandNone for /foo, got %d", got)
	}
	// /foo should have been flushed to PTY (plus the \r)
	if len(flushed) == 0 {
		t.Error("expected flushed bytes for non-command")
	}
}

func TestInterceptorPartialPrefix(t *testing.T) {
	var flushed []byte
	ic := NewInterceptor(func(b []byte) {
		flushed = append(flushed, b...)
	}, nil, "test")

	// Feed partial prefix — should hold
	got := ic.Feed([]byte("/en"))
	if got != CommandNone {
		t.Errorf("partial prefix should return CommandNone, got %d", got)
	}
	if len(flushed) != 0 {
		t.Errorf("partial prefix should not flush, got %q", flushed)
	}

	// Now complete it
	got = ic.Feed([]byte("d\r"))
	if got != CommandEnd {
		t.Errorf("completing /end should return CommandEnd, got %d", got)
	}
}

func TestInterceptorBackspace(t *testing.T) {
	var flushed []byte
	ic := NewInterceptor(func(b []byte) {
		flushed = append(flushed, b...)
	}, nil, "test")

	// Type /en then backspace
	ic.Feed([]byte("/en"))
	ic.Feed([]byte{0x7f}) // backspace

	// Buffer should now be "/e" — complete with "nd\r"
	got := ic.Feed([]byte("nd\r"))
	if got != CommandEnd {
		t.Errorf("after backspace and retype, expected CommandEnd, got %d", got)
	}
}

func TestInterceptorCtrlC(t *testing.T) {
	var flushed []byte
	ic := NewInterceptor(func(b []byte) {
		flushed = append(flushed, b...)
	}, nil, "test")

	// Start a command prefix then ctrl-C
	ic.Feed([]byte("/en"))
	flushed = nil
	ic.Feed([]byte{0x03}) // ctrl-C

	// Should have flushed the held bytes + the ctrl-C
	if len(flushed) == 0 {
		t.Error("ctrl-C should flush held buffer")
	}
}

func TestInterceptorPlainText(t *testing.T) {
	var flushed []byte
	ic := NewInterceptor(func(b []byte) {
		flushed = append(flushed, b...)
	}, nil, "test")

	got := ic.Feed([]byte("hello"))
	if got != CommandNone {
		t.Errorf("plain text should return CommandNone, got %d", got)
	}
	if string(flushed) != "hello" {
		t.Errorf("plain text should be flushed immediately, got %q", flushed)
	}
}
