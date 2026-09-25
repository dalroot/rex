package main

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// ExecResult holds the result of a command execution
type ExecResult struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// Executor runs shell commands safely via security modes
type Executor struct {
	allowlist *Allowlist
}

// NewExecutor creates an executor with the given allowlist
func NewExecutor(al *Allowlist) *Executor {
	return &Executor{allowlist: al}
}

// Run executes a command if permitted by security mode
func (e *Executor) Run(command string, timeoutSecs int) ExecResult {
	allowed, reason := e.allowlist.IsCommandAllowed(command)
	if !allowed {
		return ExecResult{
			ExitCode: -1,
			Error:    reason,
		}
	}

	if timeoutSecs <= 0 || timeoutSecs > 300 {
		timeoutSecs = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	args := splitCommand(command)
	if len(args) == 0 {
		return ExecResult{ExitCode: -1, Error: "empty command"}
	}

	start := time.Now()
	var cmd *exec.Cmd
	if strings.ContainsAny(command, "|;&><$`") || strings.Contains(command, "&&") || strings.Contains(command, "||") {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	} else {
		cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start).Milliseconds()

	result := ExecResult{
		Stdout:     strings.TrimRight(stdout.String(), "\n"),
		Stderr:     strings.TrimRight(stderr.String(), "\n"),
		DurationMs: elapsed,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			result.ExitCode = -1
			result.Error = "command timed out"
		} else {
			result.ExitCode = -1
			result.Error = err.Error()
		}
	} else {
		result.ExitCode = 0
	}

	return result
}

func splitCommand(command string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range command {
		switch {
		case inQuote && r == quoteChar:
			inQuote = false
		case !inQuote && (r == '"' || r == '\''):
			inQuote = true
			quoteChar = r
		case !inQuote && r == ' ':
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}
