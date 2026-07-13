package main

import (
	"bytes"
	"regexp"
	"strings"
)

const (
	updatesDiagnosticLimit            = 64 << 10
	updatesDiagnosticTruncationMarker = "[diagnostics truncated]"
	updatesDiagnosticRedactionMarker  = "[REDACTED]"
)

var (
	updatesCredentialAssignmentLinePattern = regexp.MustCompile(`(?i)[a-z0-9_-]*(?:token|secret|password|api[-_]?key|auth)[a-z0-9_-]*(?:\\?["'])?\s*[:=]`)
	updatesCredentialFlagLinePattern       = regexp.MustCompile(`(?i)--(?:[a-z0-9]+[-_])*(?:token|secret|password|api[-_]?key|auth(?:orization)?)(?:[-_][a-z0-9]+)*(?:\s+|=)`)
	updatesCredentialURLLinePattern        = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^/@\s]+@`)
)

type updatesDiagnosticWriter struct {
	limit   int
	total   int
	content bytes.Buffer
}

func newUpdatesDiagnosticWriter(limit int) *updatesDiagnosticWriter {
	return &updatesDiagnosticWriter{limit: limit}
}

func (w *updatesDiagnosticWriter) Write(p []byte) (int, error) {
	w.total += len(p)
	remaining := w.limit - w.content.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.content.Write(p[:remaining])
	}
	return len(p), nil
}

func (w *updatesDiagnosticWriter) String() string {
	retained := w.content.Bytes()
	if w.total <= w.limit {
		return string(retained)
	}
	prefixLimit := w.limit - len(updatesDiagnosticTruncationMarker)
	if prefixLimit < 0 {
		prefixLimit = 0
	}
	return string(retained[:prefixLimit]) + updatesDiagnosticTruncationMarker
}

func sanitizeUpdatesDiagnostic(diagnostic string) string {
	var sanitized strings.Builder
	remaining := diagnostic
	for len(remaining) > 0 {
		line := remaining
		separator := ""
		if index := strings.IndexAny(remaining, "\r\n"); index >= 0 {
			line = remaining[:index]
			separator = remaining[index : index+1]
			remaining = remaining[index+1:]
			if separator == "\r" && strings.HasPrefix(remaining, "\n") {
				separator = "\r\n"
				remaining = remaining[1:]
			}
		} else {
			remaining = ""
		}
		if updatesCredentialAssignmentLinePattern.MatchString(line) ||
			updatesCredentialFlagLinePattern.MatchString(line) ||
			updatesCredentialURLLinePattern.MatchString(line) {
			sanitized.WriteString(updatesDiagnosticRedactionMarker)
		} else {
			sanitized.WriteString(line)
		}
		sanitized.WriteString(separator)
	}
	return sanitized.String()
}

func sanitizeAndBoundUpdatesDiagnostic(diagnostic string, limit int) string {
	sanitized := sanitizeUpdatesDiagnostic(diagnostic)
	if len(sanitized) <= limit {
		return sanitized
	}
	prefixLimit := limit - len(updatesDiagnosticTruncationMarker)
	if prefixLimit < 0 {
		prefixLimit = 0
	}
	return sanitized[:prefixLimit] + updatesDiagnosticTruncationMarker
}
