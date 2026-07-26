// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Mr-Tao/btrfs-headroom/internal/model"
	"github.com/Mr-Tao/btrfs-headroom/internal/observe"
	"github.com/Mr-Tao/btrfs-headroom/internal/policy"
	"github.com/Mr-Tao/btrfs-headroom/internal/render"
)

var version = "0.1.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 64
	}
	switch args[0] {
	case "scan":
		return runScan(args[1:], false, stdout, stderr)
	case "check":
		return runScan(args[1:], true, stdout, stderr)
	case "guard":
		return runGuard(args[1:], stdout, stderr)
	case "completion":
		return runCompletion(args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		usage(stderr)
		return 64
	}
}

func runScan(args []string, healthExit bool, stdout, stderr io.Writer) int {
	command := "scan"
	if healthExit {
		command = "check"
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "human", "output format: human, json, nagios, prometheus")
	output := flags.String("output", "-", "output path, or - for stdout")
	mountInfo := flags.String("mountinfo", "/proc/self/mountinfo", "mountinfo source used for discovery and read-only state")
	if err := flags.Parse(args); err != nil {
		return 64
	}
	if *format == "nagios" && !healthExit {
		fmt.Fprintln(stderr, "nagios format is intended for the check command")
		return 64
	}

	reports, err := collectReports(flags.Args(), *mountInfo)
	if err != nil {
		fmt.Fprintf(stderr, "btrfs-headroom: %v\n", err)
		if healthExit {
			return 3
		}
		return 69
	}
	writer := stdout
	var closeOutput func() error
	var pendingOutput *os.File
	if *output != "-" {
		file, commit, err := atomicOutput(*output)
		if err != nil {
			fmt.Fprintf(stderr, "btrfs-headroom: open output: %v\n", err)
			return 73
		}
		writer = file
		pendingOutput = file
		closeOutput = commit
	}
	if err := render.Write(writer, *format, reports); err != nil {
		if pendingOutput != nil {
			name := pendingOutput.Name()
			pendingOutput.Close()
			os.Remove(name)
		}
		fmt.Fprintf(stderr, "btrfs-headroom: render: %v\n", err)
		return 74
	}
	if closeOutput != nil {
		if err := closeOutput(); err != nil {
			fmt.Fprintf(stderr, "btrfs-headroom: commit output: %v\n", err)
			return 74
		}
	}
	if healthExit {
		return render.ExitCode(reports)
	}
	return 0
}

func runGuard(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("guard", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "human", "output format: human, json, nagios, prometheus")
	failAt := flags.String("fail-at", "critical", "health threshold: warning or critical")
	unknown := flags.String("unknown", "block", "incomplete observation policy: block or allow")
	mountInfo := flags.String("mountinfo", "/proc/self/mountinfo", "mountinfo source used for discovery and read-only state")
	if err := flags.Parse(args); err != nil {
		return 64
	}
	if *failAt != "warning" && *failAt != "critical" {
		fmt.Fprintln(stderr, "btrfs-headroom: --fail-at must be warning or critical")
		return 64
	}
	if *unknown != "block" && *unknown != "allow" {
		fmt.Fprintln(stderr, "btrfs-headroom: --unknown must be block or allow")
		return 64
	}

	reports, err := collectReports(flags.Args(), *mountInfo)
	if err != nil {
		fmt.Fprintf(stderr, "btrfs-headroom: %v\n", err)
		if *unknown == "allow" {
			return 0
		}
		return 3
	}
	if err := render.Write(stdout, *format, reports); err != nil {
		fmt.Fprintf(stderr, "btrfs-headroom: render: %v\n", err)
		return 74
	}
	return guardExitCode(reports, *failAt, *unknown)
}

func collectReports(paths []string, mountInfo string) ([]model.Report, error) {
	collector := observe.NewCollector()
	collector.MountInfoPath = mountInfo
	observations, err := collector.Collect(paths)
	if err != nil {
		return nil, err
	}
	reports := make([]model.Report, 0, len(observations))
	for _, observation := range observations {
		reports = append(reports, model.Report{
			Observation: observation,
			Health:      policy.Evaluate(observation),
		})
	}
	render.SortReasons(reports)
	return reports, nil
}

func guardExitCode(reports []model.Report, failAt, unknownPolicy string) int {
	healthCode := render.ExitCode(reports)
	if healthCode == 2 {
		return 2
	}
	if unknownPolicy == "block" {
		if healthCode == 3 {
			return 3
		}
		for _, report := range reports {
			if report.Health.Confidence != "full" {
				return 3
			}
			for _, reason := range report.Health.Reasons {
				if reason.Severity == model.SeverityUnknown {
					return 3
				}
			}
		}
	}
	if healthCode == 1 && failAt == "warning" {
		return 2
	}
	return 0
}

func atomicOutput(path string) (*os.File, func() error, error) {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return nil, nil, err
	}
	commit := func() error {
		if err := file.Chmod(0o644); err != nil {
			file.Close()
			os.Remove(file.Name())
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			os.Remove(file.Name())
			return err
		}
		if err := file.Close(); err != nil {
			os.Remove(file.Name())
			return err
		}
		if err := os.Rename(file.Name(), path); err != nil {
			os.Remove(file.Name())
			return err
		}
		return nil
	}
	return file, commit, nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  btrfs-headroom scan  [--format FORMAT] [--output PATH] [--mountinfo PATH] [MOUNT...]
  btrfs-headroom check [--format FORMAT] [--output PATH] [--mountinfo PATH] [MOUNT...]
  btrfs-headroom guard [--format FORMAT] [--fail-at LEVEL] [--unknown POLICY] [--mountinfo PATH] [MOUNT...]
  btrfs-headroom completion bash|zsh|fish
  btrfs-headroom version

scan always exits zero after a valid observation. check maps health to Nagios
codes: 0 OK, 1 WARNING, 2 CRITICAL, 3 UNKNOWN. guard is an opt-in
preflight gate and never mutates the observed filesystem.`)
}
