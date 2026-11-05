package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type GitState struct {
	Branch                                                string
	Staged, Modified, Untracked, Conflicts, Ahead, Behind int
}

func readGit(dir string, timeout time.Duration) *GitState {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=normal")
	// status is read-only: do not take the optional index-refresh lock.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	data, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseGit(string(data))
}

func parseGit(data string) *GitState {
	g := &GitState{}
	var oid string
	records := strings.Split(data, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		switch {
		case strings.HasPrefix(record, "# branch.head "):
			g.Branch = strings.TrimPrefix(record, "# branch.head ")
		case strings.HasPrefix(record, "# branch.oid "):
			oid = strings.TrimPrefix(record, "# branch.oid ")
		case strings.HasPrefix(record, "# branch.ab "):
			fields := strings.Fields(strings.TrimPrefix(record, "# branch.ab "))
			if len(fields) == 2 {
				g.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
				g.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
			}
		case strings.HasPrefix(record, "? "):
			g.Untracked++
		case strings.HasPrefix(record, "u "):
			g.Conflicts++
		case strings.HasPrefix(record, "1 "), strings.HasPrefix(record, "2 "):
			if len(record) >= 4 {
				if record[2] != '.' {
					g.Staged++
				}
				if record[3] != '.' {
					g.Modified++
				}
			}
			if record[0] == '2' {
				i++ // A rename/copy has a second NUL-separated pathname.
			}
		}
	}
	if g.Branch == "(detached)" && len(oid) >= 7 {
		g.Branch = "@" + oid[:7]
	}
	if g.Branch == "" {
		return nil
	}
	return g
}

func (g *GitState) summary(ascii bool) string {
	var parts []string
	for _, item := range []struct {
		prefix string
		count  int
	}{{"+", g.Staged}, {"~", g.Modified}, {"?", g.Untracked}, {"!", g.Conflicts}} {
		if item.count > 0 {
			parts = append(parts, item.prefix+strconv.Itoa(item.count))
		}
	}
	up, down := "↑", "↓"
	if ascii {
		up, down = "^", "v"
	}
	if g.Ahead > 0 {
		parts = append(parts, up+strconv.Itoa(g.Ahead))
	}
	if g.Behind > 0 {
		parts = append(parts, down+strconv.Itoa(g.Behind))
	}
	return strings.Join(parts, " ")
}
