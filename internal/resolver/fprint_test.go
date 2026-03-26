package resolver

import (
	"bytes"
	"errors"
	"testing"
)

type errorWriter struct{}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("simulated error")
}

func TestFprintf(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var buf bytes.Buffer
		fprintf(&buf, "hello %s", "world")
		if got := buf.String(); got != "hello world" {
			t.Errorf("fprintf() = %q, want %q", got, "hello world")
		}
	})

	t.Run("error ignored", func(t *testing.T) {
		ew := &errorWriter{}
		// If this panicked, the test would fail. We just ensure it doesn't crash.
		fprintf(ew, "hello %s", "world")
	})
}

func TestFPrintln(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var buf bytes.Buffer
		fPrintln(&buf, "hello", "world")
		if got := buf.String(); got != "hello world\n" {
			t.Errorf("fPrintln() = %q, want %q", got, "hello world\n")
		}
	})

	t.Run("error ignored", func(t *testing.T) {
		ew := &errorWriter{}
		fPrintln(ew, "hello", "world")
	})
}

func TestFprint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var buf bytes.Buffer
		fprint(&buf, "hello", " world")
		if got := buf.String(); got != "hello world" {
			t.Errorf("fprint() = %q, want %q", got, "hello world")
		}
	})

	t.Run("error ignored", func(t *testing.T) {
		ew := &errorWriter{}
		fprint(ew, "hello", "world")
	})
}
