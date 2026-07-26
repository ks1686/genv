// Package export builds portable target snapshots and compatibility reports.
package export

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

const (
	ClassError      = "error"
	ClassWarning    = "warning"
	ClassSuggestion = "suggestion"
)

// ReportItem is a single export compatibility finding.
type ReportItem struct {
	Class     string `json:"class"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	PackageID string `json:"packageID,omitempty"`
}

// Report is the JSON report written alongside an exported snapshot.
type Report []ReportItem

func (r Report) HasErrors() bool {
	for _, item := range r {
		if item.Class == ClassError {
			return true
		}
	}
	return false
}

func (r Report) sorted() Report {
	out := append(Report{}, r...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Class != b.Class {
			return classRank(a.Class) < classRank(b.Class)
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.PackageID != b.PackageID {
			return a.PackageID < b.PackageID
		}
		return a.Message < b.Message
	})
	return out
}

func classRank(class string) int {
	switch class {
	case ClassError:
		return 0
	case ClassWarning:
		return 1
	case ClassSuggestion:
		return 2
	default:
		return 3
	}
}

func writeReport(path string, report Report) error {
	data, err := json.MarshalIndent(report.sorted(), "", "  ")
	if err != nil {
		return fmt.Errorf("serializing export report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
