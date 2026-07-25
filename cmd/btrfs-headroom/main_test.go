// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
)

func TestGuardExitCode(t *testing.T) {
	tests := []struct {
		name          string
		severity      model.Severity
		confidence    string
		failAt        string
		unknownPolicy string
		want          int
	}{
		{"warning allowed", model.SeverityWarning, "full", "critical", "block", 0},
		{"warning blocked", model.SeverityWarning, "full", "warning", "block", 2},
		{"critical blocked", model.SeverityCritical, "full", "critical", "block", 2},
		{"unknown blocked", model.SeverityUnknown, "full", "critical", "block", 3},
		{"unknown allowed", model.SeverityUnknown, "full", "critical", "allow", 0},
		{"partial blocked", model.SeverityOK, "partial", "critical", "block", 3},
		{"partial allowed", model.SeverityOK, "partial", "critical", "allow", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reports := []model.Report{{
				Health: model.Health{
					Severity:   test.severity,
					Confidence: test.confidence,
				},
			}}
			if got := guardExitCode(reports, test.failAt, test.unknownPolicy); got != test.want {
				t.Fatalf("guardExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestGuardBlocksUnknownReason(t *testing.T) {
	reports := []model.Report{{
		Health: model.Health{
			Severity:   model.SeverityWarning,
			Confidence: "full",
			Reasons: []model.Reason{{
				Code:     "INCOMPLETE_OBSERVATION",
				Severity: model.SeverityUnknown,
			}},
		},
	}}
	if got := guardExitCode(reports, "critical", "block"); got != 3 {
		t.Fatalf("guardExitCode() = %d, want 3", got)
	}
}

func TestAtomicOutputPublishesCompleteReadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.json")
	file, commit, err := atomicOutput(path)
	if err != nil {
		t.Fatalf("atomicOutput: %v", err)
	}
	if mode := fileMode(t, file.Name()); mode != 0o600 {
		t.Fatalf("temporary mode = %#o, want 0600", mode)
	}
	if _, err := file.WriteString("complete\n"); err != nil {
		t.Fatalf("write temporary output: %v", err)
	}
	if err := commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if mode := fileMode(t, path); mode != 0o644 {
		t.Fatalf("published mode = %#o, want 0644", mode)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read published output: %v", err)
	}
	if string(content) != "complete\n" {
		t.Fatalf("published content = %q", content)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}
