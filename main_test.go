package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContextSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, context string
		want          float64
		known         bool
	}{
		{"startup", `{}`, 0, false},
		{"explicit-zero", `{"used_percentage":0}`, 0, true},
		{"explicit-percent", `{"used_percentage":38}`, 38, true},
		{"remaining", `{"remaining_percentage":63}`, 37, true},
		{"cached-input", `{"context_window_size":200000,"current_usage":{"input_tokens":10000,"cache_read_input_tokens":60000,"cache_creation_input_tokens":10000,"output_tokens":9000}}`, 40, true},
		{"upper-bound", `{"used_percentage":120}`, 100, true},
		{"lower-bound", `{"used_percentage":-10}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := decodeSnapshot(strings.NewReader(`{"model":{"display_name":"Sonnet"},"context_window":` + tc.context + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if value, known := s.contextPercent(); value != tc.want || known != tc.known {
				t.Fatalf("percent=(%v,%v), want (%v,%v)", value, known, tc.want, tc.known)
			}
		})
	}
	for _, input := range []string{"", "null", "{}", "[]", `{"session_id":"x"} {}`, `{"model":`, strings.Repeat(" ", (4<<20)+1)} {
		if _, err := decodeSnapshot(strings.NewReader(input)); err == nil {
			t.Fatalf("expected invalid input to be rejected (length=%d)", len(input))
		}
	}
}

func TestDirectoryAndTimeDisplay(t *testing.T) {
	cfg := defaultConfig()
	if got := displayDirectory("C:/Projects/Documents/设计项目", cfg, 2); got != "…/󰈙/设计项目" {
		t.Fatalf("directory = %q", got)
	}
	if got := displayDirectory("C:/", cfg, 3); got != "C:/" {
		t.Fatalf("drive root = %q", got)
	}
	if got := displayDirectory("/", cfg, 3); got != "/" {
		t.Fatalf("root = %q", got)
	}
	if got := time.Date(2026, 11, 5, 9, 8, 0, 0, time.UTC).Format(cfg.Options.TimeLayout); got != "11-05 09:08" {
		t.Fatalf("time = %q", got)
	}
}

func TestResponsiveRibbon(t *testing.T) {
	cfg := defaultConfig()
	s := demoSnapshot()
	s.Model.DisplayName = "Claude Sonnet 超长模型名字 e\u0301"
	s.Workspace.CurrentDir = "C:/工程/中文目录/文档/一个很长很长的项目名称"
	g := &GitState{Branch: "feature/中文分支/long-name", Modified: 15, Untracked: 3, Ahead: 2}
	for _, color := range []bool{false, true} {
		for _, ascii := range []bool{false, true} {
			segments := buildSegments(s, g, cfg, time.Now(), ascii)
			for width := 1; width <= 180; width++ {
				line := render(segments, width, color, ascii)
				plain := ansiEscape.ReplaceAllString(line, "")
				if got := cellWidth.StringWidth(plain); got > width {
					t.Fatalf("width=%d color=%v ascii=%v got=%d: %q", width, color, ascii, got, plain)
				}
				if !color && strings.ContainsRune(line, '\x1b') {
					t.Fatal("plain output contains ANSI escapes")
				}
			}
		}
	}
	line := render(buildSegments(demoSnapshot(), g, cfg, time.Now(), false), 0, true, false)
	if !strings.Contains(line, cfg.Colors["user_bg"].background()) || !strings.HasSuffix(line, "\x1b[0m") {
		t.Fatal("missing palette background, rounded cap, or terminal reset")
	}
	if got := cleanText("safe\x1b[31mred\x1b[0m\x1b]0;bad-title\x07\r\n\u202eevil"); got != "saferedevil" {
		t.Fatalf("untrusted escape/control text survived: %q", got)
	}
}

func TestStartupAndMissingOptionalData(t *testing.T) {
	cfg := defaultConfig()
	s, err := decodeSnapshot(strings.NewReader(`{"model":{"display_name":"Sonnet"},"cwd":"C:/work"}`))
	if err != nil {
		t.Fatal(err)
	}
	segments := buildSegments(s, nil, cfg, time.Now(), false)
	for _, segment := range segments {
		if segment.Key == "git" || segment.Key == "usage" || segment.Key == "cost" {
			t.Fatalf("missing optional data rendered as %s", segment.Key)
		}
		if segment.Key == "context" && !strings.Contains(segment.text(), "--") {
			t.Fatal("unknown context should not be displayed as 0%")
		}
	}
	for _, pct := range []float64{0, 75, 90, 100} {
		s.Context.Used = &pct
		for _, segment := range buildSegments(s, nil, cfg, time.Now(), false) {
			if segment.Key != "context" {
				continue
			}
			want := cfg.Colors["context_low_bg"]
			if pct >= 90 {
				want = cfg.Colors["context_high_bg"]
			} else if pct >= 75 {
				want = cfg.Colors["context_mid_bg"]
			}
			if segment.BG != want {
				t.Fatalf("context background for %v%%: %v", pct, segment.BG)
			}
		}
	}
}

func TestGitPorcelainRenames(t *testing.T) {
	data := "# branch.oid 0123456789abcdef\x00# branch.head main\x00# branch.ab +2 -3\x00" +
		"1 .M ignored fields path with spaces\x00" +
		"2 R. ignored fields new-name\x00? old-name-starting-with-question-mark\x00" +
		"? untracked\nfile\x00u UU ignored fields conflict\x00"
	g := parseGit(data)
	if g == nil || g.Branch != "main" || g.Staged != 1 || g.Modified != 1 || g.Untracked != 1 || g.Conflicts != 1 || g.Ahead != 2 || g.Behind != 3 {
		t.Fatalf("unexpected Git status: %+v", g)
	}
	g = parseGit("# branch.head (detached)\x00# branch.oid 0123456789abcdef\x00")
	if g == nil || g.Branch != "@0123456" {
		t.Fatalf("detached HEAD: %+v", g)
	}
}

func TestGitRealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-b", "main")
	write("tracked.txt", "initial\n")
	run("add", "tracked.txt")
	run("-c", "user.name=ccstarline", "-c", "user.email=ccstarline@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath="+filepath.Join(dir, "no-hooks"), "commit", "-m", "initial")
	write("tracked.txt", "changed\n")
	write("staged.txt", "staged\n")
	run("add", "staged.txt")
	write("untracked.txt", "untracked\n")
	g := readGit(dir, 5*time.Second)
	if g == nil || g.Branch != "main" || g.Staged != 1 || g.Modified != 1 || g.Untracked != 1 {
		t.Fatalf("real Git status: %+v", g)
	}
	if got := readGit(t.TempDir(), time.Second); got != nil {
		t.Fatalf("non-repository produced Git status: %+v", got)
	}
}
