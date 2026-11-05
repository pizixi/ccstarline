package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// version is overridden at build time with
// -ldflags "-X main.version=..." by the release workflow.
var version = "1.0.1"

// Claude Code supplies one complete JSON snapshot on stdin for each repaint.
type Snapshot struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Model     struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
		ProjectDir string `json:"project_dir"`
	} `json:"workspace"`
	Context struct {
		Size      int64    `json:"context_window_size"`
		Used      *float64 `json:"used_percentage"`
		Remaining *float64 `json:"remaining_percentage"`
		Usage     *struct {
			Input int64 `json:"input_tokens"`
			Read  int64 `json:"cache_read_input_tokens"`
			Write int64 `json:"cache_creation_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	Cost *struct {
		USD *float64 `json:"total_cost_usd"`
	} `json:"cost"`
	Limits struct {
		FiveHour *Limit `json:"five_hour"`
		SevenDay *Limit `json:"seven_day"`
	} `json:"rate_limits"`
}

type Limit struct {
	Used *float64 `json:"used_percentage"`
}

func decodeSnapshot(r io.Reader) (Snapshot, error) {
	var s Snapshot
	// Bound input without silently accepting a truncated but valid prefix.
	data, err := io.ReadAll(io.LimitReader(r, (4<<20)+1))
	if err != nil {
		return s, err
	}
	if len(data) > 4<<20 {
		return s, errors.New("stdin JSON exceeds 4 MiB")
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("read Claude Code stdin JSON: %w", err)
	}
	if s.Model.DisplayName == "" && s.Model.ID == "" && s.SessionID == "" && s.CWD == "" && s.Workspace.CurrentDir == "" {
		return s, errors.New("stdin does not contain a Claude Code session snapshot")
	}
	return s, nil
}

func (s Snapshot) directory() string {
	for _, dir := range []string{s.Workspace.CurrentDir, s.CWD, s.Workspace.ProjectDir} {
		if dir != "" {
			if absolute, err := filepath.Abs(dir); err == nil {
				return absolute
			}
			return dir
		}
	}
	dir, _ := os.Getwd()
	return dir
}

func demoSnapshot() Snapshot {
	const demo = `{
	  "session_id":"b63a87b9-7ac5-4ab4-9229-c80f82daaef1",
	  "model":{"display_name":"Sonnet 4.5"},
	  "workspace":{"current_dir":"C:/Projects/ccstarline"},
	  "context_window":{"context_window_size":200000,"used_percentage":38,"current_usage":{"input_tokens":16000,"cache_read_input_tokens":60000}},
	  "cost":{"total_cost_usd":0.128},
	  "rate_limits":{"five_hour":{"used_percentage":24},"seven_day":{"used_percentage":12}}
	}`
	s, _ := decodeSnapshot(strings.NewReader(demo))
	if runtime.GOOS != "windows" {
		s.Workspace.CurrentDir = "/home/dev/Projects/ccstarline"
	}
	return s
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ccstarline:", err)
		os.Exit(1)
	}
}

func run() error {
	preview := flag.Bool("preview", false, "render a sample session without reading stdin")
	noColor := flag.Bool("no-color", false, "disable ANSI colors")
	forceColor := flag.Bool("color", false, "force ANSI colors, including when NO_COLOR is set")
	ascii := flag.Bool("ascii", false, "use ASCII symbols and separators")
	width := flag.Int("width", 0, "maximum display columns (0 reads COLUMNS; unset means unlimited)")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println("ccstarline", version)
		return nil
	}
	if flag.NArg() != 0 || *width < 0 {
		return errors.New("unexpected arguments or negative --width; run --help")
	}
	if *forceColor && *noColor {
		return errors.New("--color and --no-color cannot be used together")
	}
	cfg := defaultConfig()
	var s Snapshot
	var git *GitState
	var err error
	if *preview {
		s = demoSnapshot()
		git = &GitState{Branch: "main", Staged: 2, Modified: 1, Untracked: 1, Ahead: 2}
	} else {
		if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return errors.New("expected Claude Code JSON on stdin; use --preview for a demo")
		}
		s, err = decodeSnapshot(os.Stdin)
		if err != nil {
			return err
		}
		if cfg.hasModule("git") {
			git = readGit(s.directory(), time.Duration(cfg.Options.GitTimeoutMS)*time.Millisecond)
		}
	}
	if *width == 0 {
		*width, _ = strconv.Atoi(os.Getenv("COLUMNS"))
	}
	color := !*noColor && (*forceColor || os.Getenv("NO_COLOR") == "")
	line := render(buildSegments(s, git, cfg, time.Now(), *ascii), *width, color, *ascii)
	fmt.Fprintln(os.Stdout, line)
	return nil
}
