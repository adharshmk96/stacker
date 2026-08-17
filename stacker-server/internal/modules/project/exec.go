package project

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Command is one process a deploy runs.
type Command struct {
	// Name and Args are passed to exec directly — never through a shell, so a
	// repository URL or an environment value can hold any character without
	// becoming a quoting problem.
	Name string
	Args []string
	// Dir is the workspace the command runs in.
	Dir string
	// Env replaces the process environment entirely, so the parent's variables
	// cannot leak into a build.
	Env []string
}

// String renders the command the way it is echoed into the run log.
func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Sink receives one line of output at a time. It is how the run log fills up
// while a command is still going, which is what makes the log view live rather
// than a dump at the end.
type Sink func(line string)

// Exec runs a command and streams its output. A non-nil error carries the last
// lines of output, since a deploy that failed needs a reason in the toast and
// not just an exit code.
type Exec func(ctx context.Context, cmd Command, sink Sink) error

// execCommand is the real implementation.
//
// stdout and stderr are merged into one stream: docker writes progress to
// stderr and results to stdout, and a log that interleaves them is the log a
// person would have seen in a terminal.
func execCommand(ctx context.Context, cmd Command, sink Sink) error {
	process := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	process.Dir = cmd.Dir
	process.Env = cmd.Env

	pipe, err := process.StdoutPipe()
	if err != nil {
		return err
	}
	process.Stderr = process.Stdout

	if err := process.Start(); err != nil {
		return fmt.Errorf("could not start %s: %w", cmd.Name, err)
	}

	tail := newTail(8)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scan(pipe, func(line string) {
			tail.add(line)
			sink(line)
		})
	}()
	wg.Wait()

	if err := process.Wait(); err != nil {
		if reason := tail.String(); reason != "" {
			return fmt.Errorf("%s failed: %s", cmd.Name, reason)
		}
		return fmt.Errorf("%s failed: %w", cmd.Name, err)
	}
	return nil
}

// scan reads lines without a length limit on any one of them: a docker build
// can emit a single very long line, and bufio.Scanner would abort the whole
// stream when it did.
func scan(source io.Reader, emit func(string)) {
	reader := bufio.NewReader(source)
	var builder strings.Builder

	for {
		chunk, err := reader.ReadString('\n')
		// Docker's progress output uses carriage returns to repaint a line;
		// splitting on them too keeps the log readable instead of one long line.
		chunk = strings.ReplaceAll(chunk, "\r", "\n")
		builder.WriteString(chunk)

		for {
			text := builder.String()
			index := strings.IndexByte(text, '\n')
			if index == -1 {
				break
			}
			if line := strings.TrimRight(text[:index], " \t"); line != "" {
				emit(line)
			}
			builder.Reset()
			builder.WriteString(text[index+1:])
		}

		if err != nil {
			if line := strings.TrimSpace(builder.String()); line != "" {
				emit(line)
			}
			return
		}
	}
}

// tail keeps the last few lines of a command, which is what a failure reports.
type tail struct {
	limit int
	lines []string
}

func newTail(limit int) *tail { return &tail{limit: limit} }

func (t *tail) add(line string) {
	if line = strings.TrimSpace(line); line == "" {
		return
	}
	t.lines = append(t.lines, line)
	if len(t.lines) > t.limit {
		t.lines = t.lines[len(t.lines)-t.limit:]
	}
}

func (t *tail) String() string { return strings.Join(t.lines, " · ") }
