package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// callResult is the structured payload returned for every tool call, mirroring
// gogcli's result shape.
type callResult struct {
	Tool      string `json:"tool"`
	Service   string `json:"service"`
	Risk      Risk   `json:"risk"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Stdout    any    `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
	TimedOut  bool   `json:"timed_out,omitempty"`
}

// runner executes a resolved asc invocation and returns stdout, stderr and the
// process exit code. It is an interface so tests can stub subprocess execution.
type runner interface {
	run(ctx context.Context, argv []string) (stdout, stderr []byte, exitCode int, err error)
}

// execRunner runs the asc binary at binPath as a subprocess.
type execRunner struct {
	binPath string
}

func (r execRunner) run(ctx context.Context, argv []string) ([]byte, []byte, int, error) {
	cmd := exec.CommandContext(ctx, r.binPath, argv...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil
		} else {
			exitCode = -1
		}
	}
	return []byte(stdout.String()), []byte(stderr.String()), exitCode, err
}

// buildArgv turns a tool plus a JSON arguments object into the asc argv
// (excluding the binary name). Flags are emitted as --name value, bool flags
// as a bare --name when true, and the optional "args" array is appended as
// positional arguments. Flag order is deterministic (sorted) for reproducible
// invocations.
func buildArgv(t Tool, arguments map[string]any) ([]string, error) {
	argv := append([]string{}, t.Path...)
	boolFlags := t.boolFlagNames()

	names := make([]string, 0, len(arguments))
	for k := range arguments {
		if k != "args" {
			names = append(names, k)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		val := arguments[name]
		if _, isBool := boolFlags[name]; isBool {
			b, err := toBool(val)
			if err != nil {
				return nil, fmt.Errorf("flag --%s: %w", name, err)
			}
			if b {
				argv = append(argv, "--"+name)
			}
			continue
		}
		s, err := toScalarString(val)
		if err != nil {
			return nil, fmt.Errorf("flag --%s: %w", name, err)
		}
		argv = append(argv, "--"+name, s)
	}

	if raw, ok := arguments["args"]; ok {
		positionals, err := toStringSlice(raw)
		if err != nil {
			return nil, fmt.Errorf("args: %w", err)
		}
		argv = append(argv, positionals...)
	}
	return argv, nil
}

func toBool(v any) (bool, error) {
	switch t := v.(type) {
	case bool:
		return t, nil
	case string:
		b, err := strconv.ParseBool(t)
		if err != nil {
			return false, fmt.Errorf("expected boolean, got %q", t)
		}
		return b, nil
	case nil:
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean, got %T", v)
	}
}

func toScalarString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case float64:
		// JSON numbers decode to float64; render integers without a decimal.
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10), nil
		}
		return strconv.FormatFloat(t, 'g', -1, 64), nil
	case json.Number:
		return t.String(), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("expected scalar value, got %T", v)
	}
}

func toStringSlice(v any) ([]string, error) {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, err := toScalarString(item)
			if err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return t, nil
	case string:
		if t == "" {
			return nil, nil
		}
		return []string{t}, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("expected array of strings, got %T", v)
	}
}

// execute resolves arguments, runs the tool (unless dry-run), and assembles a
// structured callResult. stdout is parsed as JSON when valid, otherwise
// returned as a (possibly truncated) string.
func (s *Server) execute(ctx context.Context, t Tool, arguments map[string]any) (callResult, error) {
	argv, err := buildArgv(t, arguments)
	if err != nil {
		return callResult{}, err
	}

	res := callResult{
		Tool:    t.Name,
		Service: t.Service,
		Risk:    t.Risk,
		Command: "asc " + strings.Join(argv, " "),
	}

	if s.dryRun {
		res.DryRun = true
		res.Stdout = ""
		return res, nil
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if s.timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	stdout, stderr, exitCode, runErr := s.runner.run(runCtx, argv)
	if runErr != nil {
		return callResult{}, runErr
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		if exitCode == 0 {
			exitCode = -1
		}
	}

	res.ExitCode = exitCode
	stderrTrimmed, _ := truncate(stderr, s.maxOutputBytes)
	res.Stderr = string(stderrTrimmed)

	parsed, truncated := decodeStdout(stdout, s.maxOutputBytes)
	res.Stdout = parsed
	res.Truncated = truncated
	return res, nil
}

// decodeStdout returns parsed JSON when stdout is valid JSON within the byte
// limit; otherwise it returns a (possibly truncated) string and reports
// whether truncation occurred.
func decodeStdout(stdout []byte, maxBytes int) (any, bool) {
	trimmed, wasTruncated := truncate(stdout, maxBytes)
	if !wasTruncated {
		var js any
		if err := json.Unmarshal(stdout, &js); err == nil {
			return js, false
		}
	}
	return string(trimmed), wasTruncated
}

// truncate caps b to maxBytes (maxBytes <= 0 means unlimited) and reports
// whether any bytes were dropped.
func truncate(b []byte, maxBytes int) ([]byte, bool) {
	if maxBytes > 0 && len(b) > maxBytes {
		return b[:maxBytes], true
	}
	return b, false
}
