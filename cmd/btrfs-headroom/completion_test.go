// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestCompletionCommand(t *testing.T) {
	tests := []struct {
		shell   string
		marker  string
		options []string
	}{
		{
			"bash",
			"complete -F _btrfs_headroom btrfs-headroom",
			[]string{
				"-h", "--help", "-version", "--version",
				"--format", "--output", "--mountinfo", "--fail-at", "--unknown",
			},
		},
		{
			"zsh",
			"#compdef btrfs-headroom",
			[]string{
				"-h", "--help", "-version", "--version",
				"--format", "--output", "--mountinfo", "--fail-at", "--unknown",
			},
		},
		{
			"fish",
			"complete -c btrfs-headroom",
			[]string{
				"-s h", "-l help", "-o version", "-l version",
				"-l format", "-l output", "-l mountinfo", "-l fail-at", "-l unknown",
			},
		},
	}
	required := []string{
		"scan", "check", "guard", "completion", "help", "version",
		"human", "json", "nagios", "prometheus",
		"warning", "critical", "block", "allow",
		"bash", "zsh", "fish",
	}

	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if got := run([]string{"completion", test.shell}, &stdout, &stderr); got != 0 {
				t.Fatalf("run() = %d, want 0; stderr = %q", got, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			script := stdout.String()
			if !strings.Contains(script, test.marker) {
				t.Errorf("completion does not contain shell marker %q", test.marker)
			}
			for _, item := range required {
				if !strings.Contains(script, item) {
					t.Errorf("completion does not contain %q", item)
				}
			}
			for _, option := range test.options {
				if !strings.Contains(script, option) {
					t.Errorf("completion does not contain option %q", option)
				}
			}
		})
	}
}

func TestCompletionCommandRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing shell", []string{"completion"}},
		{"unsupported shell", []string{"completion", "powershell"}},
		{"extra argument", []string{"completion", "bash", "extra"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if got := run(test.args, &stdout, &stderr); got != 64 {
				t.Fatalf("run() = %d, want 64", got)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), "completion bash|zsh|fish") {
				t.Fatalf("stderr = %q, want completion usage", stderr.String())
			}
		})
	}
}

func TestCompletionCommandReportsWriteFailure(t *testing.T) {
	var stderr bytes.Buffer
	if got := runCompletion([]string{"bash"}, failingWriter{}, &stderr); got != 74 {
		t.Fatalf("runCompletion() = %d, want 74", got)
	}
	if !strings.Contains(stderr.String(), "write completion") {
		t.Fatalf("stderr = %q, want write failure", stderr.String())
	}
}

func TestVersionCanBeInjected(t *testing.T) {
	original := version
	version = "1.2.3-test"
	t.Cleanup(func() {
		version = original
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := run([]string{"version"}, &stdout, &stderr); got != 0 {
		t.Fatalf("run() = %d, want 0", got)
	}
	if got := stdout.String(); got != "1.2.3-test\n" {
		t.Fatalf("stdout = %q, want injected version", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("test write failure")
}
