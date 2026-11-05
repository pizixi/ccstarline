package main

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

type RGB struct{ R, G, B uint8 }

func rgb(value uint32) RGB {
	return RGB{uint8(value >> 16), uint8(value >> 8), uint8(value)}
}

func (c RGB) foreground() string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B) }
func (c RGB) background() string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c.R, c.G, c.B) }

var ansiEscape = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|[@-_])")
var cellWidth = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false // Nerd Font private-use glyphs occupy one terminal cell.
	return c
}()

func cleanText(s string) string {
	s = ansiEscape.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}
		return r
	}, s)
}

func shorten(s string, width int, ascii bool) string {
	marker := "…"
	if ascii {
		marker = "~"
	}
	return cellWidth.Truncate(cleanText(s), width, marker)
}

type Segment struct {
	Key     string
	FG, BG  RGB
	Texts   []string
	joined  bool
	variant int
}

func (s Segment) text() string { return s.Texts[s.variant] }

func render(input []Segment, width int, color, ascii bool) string {
	segments := append([]Segment(nil), input...)
	fits := func() bool { return width <= 0 || ribbonWidth(segments, color, ascii) <= width }
	if fits() {
		return paint(segments, color, ascii)
	}
	// Compact text before removing information. Always measure before emitting ANSI.
	for pass := 0; pass < 2; pass++ {
		for _, key := range []string{"directory", "git", "time", "model", "context", "usage", "tokens", "session"} {
			for i := range segments {
				if segments[i].Key == key && segments[i].variant+1 < len(segments[i].Texts) {
					segments[i].variant++
				}
			}
			if fits() {
				return paint(segments, color, ascii)
			}
		}
	}
	for _, key := range []string{"time", "session", "tokens", "usage", "cost", "git", "directory"} {
		for i, segment := range segments {
			if segment.Key == key {
				segments = append(segments[:i], segments[i+1:]...)
				break
			}
		}
		if fits() {
			return paint(segments, color, ascii)
		}
	}
	// At very small widths keep a readable model/context label without a broken ribbon.
	var texts []string
	for _, segment := range segments {
		texts = append(texts, segment.text())
	}
	return shorten(strings.Join(texts, " "), width, ascii)
}

func ribbonWidth(segments []Segment, color, ascii bool) int {
	if len(segments) == 0 {
		return 0
	}
	w := 0
	for _, segment := range segments {
		w += cellWidth.StringWidth(segment.text())
	}
	joined := 0
	for i := 1; i < len(segments); i++ {
		if segments[i].joined {
			joined++
		}
	}
	boundaries := len(segments) - 1
	if !color {
		return w + 3*(boundaries-joined) + joined
	}
	if ascii {
		return w + boundaries + boundaries - joined
	}
	return w + boundaries + (boundaries - joined) + 2
}

func paint(segments []Segment, color, ascii bool) string {
	if len(segments) == 0 {
		return ""
	}
	var out strings.Builder
	if !color || ascii {
		for i, segment := range segments {
			if i > 0 {
				if !color {
					if segment.joined {
						out.WriteByte(' ')
					} else {
						out.WriteString(" | ")
					}
				} else if !segment.joined {
					out.WriteByte(' ')
				}
			}
			if color {
				pad := ""
				if i > 0 {
					pad = " "
				}
				out.WriteString(segment.FG.foreground() + segment.BG.background() + pad + segment.text() + "[0m")
			} else {
				out.WriteString(segment.text())
			}
		}
		return out.String()
	}
	out.WriteString("[0m" + segments[0].BG.foreground() + "")
	for i, segment := range segments {
		pad := ""
		if i > 0 {
			pad = " "
		}
		out.WriteString(segment.FG.foreground() + segment.BG.background() + pad + segment.text())
		if i+1 < len(segments) {
			if !segments[i+1].joined {
				out.WriteString(segment.BG.foreground() + segments[i+1].BG.background() + "")
			}
		} else {
			out.WriteString("[0m" + segment.BG.foreground() + "[0m")
		}
	}
	return out.String()
}
