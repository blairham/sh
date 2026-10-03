// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"

	"github.com/blairham/sh/internal/tty"
	"github.com/blairham/sh/syntax"
)

// One shell asks before `rm *` at an interactive prompt, unless
// `rmstarsilent` is on. Measured 2026-10-02 on zsh 5.9.2 under `-f` through
// a pseudo-terminal, in a directory holding `a`, `b` and `c` (#5155):
//
//	rm *          zsh: sure you want to delete all 3 files in /dir [yn]? ␇
//	rm d/*        … all the files in /dir/d … when d does not exist
//	rm ./*        … in /dir/. … — the prefix as written, not tidied
//	rm /*         … all 16 files in / …
//	rm *.x, ls *, rm '*'    no question
//	rm -f *, rm a *, \rm *, command rm *    the question
//
// The bell goes out with the question. `y` or `Y` runs the command, `n`,
// `N` or a newline gives up the whole line — `rm *; print st=$?` prints
// nothing — and any other key is another bell. The answer is echoed as `y` or
// `n` and a newline, and nothing is asked of a shell that is not interactive.

// SetRmStarSilent turns one shell's `rmstarsilent` on and off.
func (r *Runner) SetRmStarSilent(on bool) { r.rmStarSilent = on }

// RmStarSilent reports it.
func (r *Runner) RmStarSilent() bool { return r.rmStarSilent }

// SetRmStarAsks says the dialect asks before `rm *` at all.
func (r *Runner) SetRmStarAsks(on bool) { r.rmStarAsks = on }

// declinesRmStar asks the question where this command earns it, and reports
// whether the answer was no.
func (r *Runner) declinesRmStar(c *syntax.SimpleCmd) bool {
	if !r.rmStarAsks || r.rmStarSilent || !r.Interactive || len(c.Args) == 0 {
		return false
	}
	// The command word as written, quoting and all taken away: `\rm *` is
	// asked about too.
	name := writtenText(c.Args[0])
	if name == "command" && len(c.Args) > 1 {
		name = writtenText(c.Args[1])
	}
	if name != "rm" {
		return false
	}
	prefix, ok := rmStarWord(c)
	if !ok {
		return false
	}
	f, held := r.terminal()
	if !held {
		return false
	}
	dir := strings.TrimSuffix(prefix, "/")
	switch {
	case prefix == "":
		dir = r.Dir
	case strings.HasPrefix(prefix, "/"):
		if dir == "" {
			dir = "/"
		}
	default:
		dir = r.Dir + "/" + dir
	}
	what := "all the files"
	if entries, err := r.readDir(dir); err == nil {
		n := 0
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				n++
			}
		}
		what = fmt.Sprintf("all %d files", n)
	}
	_, _ = fmt.Fprintf(f, "%s: sure you want to delete %s in %s [yn]? \a", r.name(), what, dir)
	mode, err := tty.Raw(f)
	if err == nil {
		defer func() { _ = mode.Restore() }()
	}
	var b [1]byte
	yes := false
	for {
		if n, err := f.Read(b[:]); n == 0 || err != nil {
			break
		}
		if b[0] == 'y' || b[0] == 'Y' {
			yes = true
			break
		}
		if b[0] == 'n' || b[0] == 'N' || b[0] == '\n' || b[0] == '\r' {
			break
		}
		_, _ = f.Write([]byte{'\a'})
	}
	answer := "n"
	if yes {
		answer = "y"
	}
	_, _ = f.Write([]byte(answer + "\r\n"))
	return !yes
}

// writtenText is a word's literal text with its quoting taken away, and
// empty for a word holding anything but literal text.
func writtenText(w *syntax.Word) string {
	text := ""
	for _, s := range w.Spans {
		if s.Kind != syntax.Literal {
			return ""
		}
		text += s.Value
	}
	return text
}

// rmStarWord finds the word that earns the question — `*` or a prefix
// ending in `/*`, written unquoted — and returns its prefix.
func rmStarWord(c *syntax.SimpleCmd) (string, bool) {
	for _, w := range c.Args {
		text := ""
		for _, s := range w.Spans {
			if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
				text = ""
				break
			}
			text += s.Value
		}
		if text == "*" {
			return "", true
		}
		if strings.HasSuffix(text, "/*") && !strings.Contains(strings.TrimSuffix(text, "*"), "*") {
			return strings.TrimSuffix(text, "*"), true
		}
	}
	return "", false
}
