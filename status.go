package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

func clampPercent(n float64) float64 { return math.Max(0, math.Min(100, n)) }

func (s Snapshot) contextPercent() (float64, bool) {
	if s.Context.Used != nil {
		return clampPercent(*s.Context.Used), true
	}
	if s.Context.Remaining != nil {
		return clampPercent(100 - *s.Context.Remaining), true
	}
	if s.Context.Usage != nil && s.Context.Size > 0 {
		u := s.Context.Usage
		return clampPercent(float64(u.Input+u.Read+u.Write) / float64(s.Context.Size) * 100), true
	}
	return 0, false
}

func displayDirectory(path string, cfg Config, limit int) string {
	path = strings.ReplaceAll(path, "\\", "/")
	if home, err := os.UserHomeDir(); err == nil {
		home = strings.TrimRight(strings.ReplaceAll(home, "\\", "/"), "/")
		prefix, target := path, home
		if os.PathSeparator == '\\' {
			prefix, target = strings.ToLower(prefix), strings.ToLower(target)
		}
		if prefix == target || strings.HasPrefix(prefix, target+"/") {
			path = "~" + path[len(home):]
		}
	}
	leadingSlash := strings.HasPrefix(path, "/")
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	truncated := len(parts) > limit
	if truncated {
		parts = parts[len(parts)-limit:]
	}
	for i, part := range parts {
		if replacement, ok := cfg.Directory.Substitutions[part]; ok {
			parts[i] = strings.TrimSpace(replacement)
		}
	}
	result := strings.Join(parts, "/")
	if truncated {
		result = cfg.Directory.Symbol + result
	} else if leadingSlash {
		result = "/" + result
	} else if len(parts) == 1 && strings.HasSuffix(parts[0], ":") {
		result += "/"
	}
	if result == "" {
		return "/"
	}
	return cleanText(result)
}

func buildSegments(s Snapshot, git *GitState, cfg Config, now time.Time, ascii bool) []Segment {
	if ascii {
		cfg.Directory.Symbol = ".../"
		cfg.Directory.Substitutions = nil
	}
	icon := func(nerd, fallback string) string {
		if ascii {
			return fallback
		}
		return nerd
	}
	var segments []Segment
	add := func(key, bg, fg string, texts ...string) {
		for i := range texts {
			texts[i] = strings.TrimSpace(cleanText(texts[i]))
		}
		segments = append(segments, Segment{Key: key, BG: cfg.Colors[bg], FG: cfg.Colors[fg], Texts: texts, joined: cfg.joined(key)})
	}
	for _, name := range cfg.Options.Modules {
		switch name {
		case "model":
			model := s.Model.DisplayName
			if model == "" {
				model = s.Model.ID
			}
			if model == "" {
				model = "Claude"
			}
			model = cleanText(strings.TrimPrefix(model, "Claude "))
			add(name, "user_bg", "light", icon("󰍛", "AI")+" "+model, icon("󰍛", "AI")+" "+shorten(model, 20, ascii), shorten(model, 14, ascii))
		case "directory":
			dir := s.directory()
			add(name, "directory_bg", "dark", icon("", "dir")+" "+displayDirectory(dir, cfg, cfg.Directory.Length), icon("", "dir")+" "+displayDirectory(dir, cfg, 1), shorten(displayDirectory(dir, cfg, 1), 16, ascii))
		case "git":
			if git == nil {
				continue
			}
			branch := cleanText(git.Branch)
			prefix := icon(cleanText(cfg.GitBranch.Symbol), "git") + " "
			full := prefix + branch
			if summary := git.summary(ascii); summary != "" {
				full += " " + summary
			} else {
				full += icon(" ✓", " clean")
			}
			add(name, "git_bg", "dark", full, prefix+shorten(branch, 20, ascii), prefix+shorten(branch, 12, ascii))
		case "context":
			pct, ok := s.contextPercent()
			if !ok {
				add(name, "language_bg", "dark", icon("󰓅", "ctx")+" --")
				continue
			}
			bg, symbol := "context_low_bg", icon("󰓅", "ctx")
			if pct >= 90 {
				bg, symbol = "context_high_bg", icon("󰀪", "ctx!")
			} else if pct >= 75 {
				bg = "context_mid_bg"
			}
			filled := int(math.Round(pct / 100 * float64(cfg.Options.BarWidth)))
			solid, empty := "▰", "▱"
			if ascii {
				solid, empty = "#", "-"
			}
			bar := strings.Repeat(solid, filled) + strings.Repeat(empty, cfg.Options.BarWidth-filled)
			percent := fmt.Sprintf("%.0f%%", pct)
			add(name, bg, "dark", symbol+" "+bar+" "+percent, symbol+" "+percent, percent)
		case "usage":
			var full, compact []string
			for _, limit := range []struct {
				label string
				value *Limit
			}{{"5h", s.Limits.FiveHour}, {"7d", s.Limits.SevenDay}} {
				if limit.value != nil && limit.value.Used != nil {
					pct := clampPercent(*limit.value.Used)
					full = append(full, fmt.Sprintf("%s %.0f%%", limit.label, pct))
					compact = append(compact, fmt.Sprintf("%s %.0f%%", limit.label, pct))
				}
			}
			if len(full) > 0 {
				separator := " · "
				if ascii {
					separator = " / "
				}
				add(name, "docker_bg", "dark", icon("󰥔 ", "")+strings.Join(full, separator), strings.Join(compact, " "))
			}
		case "cost":
			if s.Cost != nil && s.Cost.USD != nil {
				cost := math.Max(0, *s.Cost.USD)
				add(name, "docker_bg", "dark", icon(" ", "$")+fmt.Sprintf("%.3f", cost))
			}
		case "time":
			add(name, "time_bg", "dark", icon(" ", "")+now.Format(cfg.Options.TimeLayout), icon(" ", "")+now.Format("15:04"))
		case "tokens":
			if u := s.Context.Usage; u != nil && s.Context.Size > 0 {
				text := formatTokens(u.Input+u.Read+u.Write) + "/" + formatTokens(s.Context.Size)
				add(name, "tokens_bg", "dark", icon("󰆼", "tok")+" "+text, text)
			}
		case "session":
			if s.SessionID != "" {
				add(name, "user_bg", "light", icon("󰘳", "sid")+" "+shorten(s.SessionID, 8, ascii))
			}
		}
	}
	return segments
}

func formatTokens(n int64) string {
	if n >= 1_000_000 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000_000), ".0") + "M"
	}
	if n >= 1000 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1000), ".0") + "k"
	}
	return fmt.Sprint(n)
}
