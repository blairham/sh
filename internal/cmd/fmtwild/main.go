// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command wild formats every shell script it can find and holds each result
// to the formatter's three promises: the tree survives, the comments survive,
// and a second pass changes nothing. It never runs anything.
//
// Scripts the substrate cannot parse are counted and set aside — they are
// parser facts, not formatter facts. Everything else must verify.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"

	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
)

var (
	dirs    = flag.String("dirs", "/opt/homebrew/bin:/usr/local/bin:/etc", "colon-separated roots to sweep")
	verbose = flag.Bool("v", false, "list every failing path under its cause")
	depth   = flag.Int("depth", 6, "how deep to walk")
	explain = flag.String("explain", "", "show one script's failure in detail and exit")
)

// explainOne formats one script and reports exactly how it fails: the
// reparse error with the offending output lines, or which property broke.
func explainOne(path string) {
	src, d, st, ok := read(path)
	if !ok {
		fmt.Println("not recognized as a shell script")
		return
	}
	f, err := syntax.Parse(src, d)
	if err != nil {
		fmt.Println("input does not parse:", err)
		return
	}
	cs := comments.Recover(src, f)
	out := printer.Format(src, f, cs, st)
	fOut, err := syntax.Parse(out, d)
	if err != nil {
		fmt.Println("output does not parse:", err)
		lines := strings.Split(out, "\n")
		var at int
		if e, ok := err.(*syntax.Error); ok {
			at = e.Pos.Line
		}
		for i := max(0, at-4); i < min(len(lines), at+3); i++ {
			fmt.Printf("%5d| %s\n", i+1, lines[i])
		}
		return
	}
	if why, ok := syntax.SameProgram(f, fOut); !ok {
		fmt.Println("the program changed at", why)
		return
	}
	if !sameComments(cs, comments.Recover(out, fOut)) {
		fmt.Println("comments changed")
		return
	}
	if again := printer.Format(out, fOut, comments.Recover(out, fOut), st); again != out {
		reportFirstDiff("not idempotent", out, again)
		return
	}
	fmt.Println("verifies clean")
}

func reportFirstDiff(what, a, b string) {
	fmt.Println(what)
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			fmt.Printf("first difference at line %d:\n", i+1)
			for j := max(0, i-2); j < min(len(al), i+4); j++ {
				fmt.Printf("  before| %s\n", al[j])
			}
			for j := max(0, i-2); j < min(len(bl), i+4); j++ {
				fmt.Printf("  after | %s\n", bl[j])
			}
			return
		}
	}
}

func main() {
	flag.Parse()
	if *explain != "" {
		explainOne(*explain)
		return
	}
	var parsed, skipped int
	fails := map[string][]string{}
	seen := map[string]bool{}
	for _, root := range strings.Split(*dirs, ":") {
		if root == "" {
			continue
		}
		walkRoot(root, seen, &parsed, &skipped, fails)
	}
	fmt.Printf("parsed and verified: %d\nnot parseable (set aside): %d\n", parsed, skipped)
	if len(fails) == 0 {
		fmt.Println("failures: none")
		return
	}
	causes := make([]string, 0, len(fails))
	for c := range fails {
		causes = append(causes, c)
	}
	sort.Slice(causes, func(i, j int) bool { return len(fails[causes[i]]) > len(fails[causes[j]]) })
	for _, c := range causes {
		fmt.Printf("FAIL %-22s %d script(s)   e.g. %s\n", c, len(fails[c]), fails[c][0])
		if *verbose {
			for _, p := range fails[c] {
				fmt.Println("   ", p)
			}
		}
	}
	os.Exit(1)
}

func walkRoot(root string, seen map[string]bool, parsed, skipped *int, fails map[string][]string) {
	base := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(path, string(os.PathSeparator))-base >= *depth {
				return filepath.SkipDir
			}
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || seen[resolved] || denied(resolved) {
			return nil
		}
		seen[resolved] = true
		src, dialect, layout, ok := read(resolved)
		if !ok {
			return nil
		}
		f, err := syntax.Parse(src, dialect)
		if err != nil {
			*skipped++
			return nil
		}
		*parsed++
		if cause := verify(src, f, dialect, layout); cause != "" {
			fails[cause] = append(fails[cause], path)
		}
		return nil
	})
}

// denied skips a shell's own distribution — CLEANROOM.md's red list. The
// tell is a package root above the shell's name or a version below it;
// site-functions and third-party completions carry neither and stay in.
func denied(path string) bool {
	for _, m := range []string{
		"/Cellar/zsh/", "/Cellar/bash/", "/Cellar/dash/", "/Cellar/ksh",
		"/opt/zsh/", "/opt/bash/", "/opt/dash/",
		"/share/zsh/5", "/usr/share/zsh/",
	} {
		if strings.Contains(path, m) {
			return true
		}
	}
	return false
}

// read decides whether a file is a shell script and in which dialect, by the
// rules the substrate's own wild sweep uses: shebang first, then the name.
// read also decides the layout, because the dialect and its layout are one
// decision: a file read as zsh is laid out as zsh.
func read(path string) (string, syntax.Dialect, syntax.Style, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1<<20 {
		return "", syntax.Dialect{}, syntax.Style{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", syntax.Dialect{}, syntax.Style{}, false
	}
	src := string(b)
	first, _, _ := strings.Cut(src, "\n")
	switch {
	case strings.HasPrefix(first, "#!"):
		switch {
		case strings.Contains(first, "bash"):
			return src, bash.Dialect(), bash.Style(), true
		case strings.Contains(first, "zsh"):
			return src, zsh.Dialect(), zsh.Style(), true
		case strings.Contains(first, "dash"):
			return src, dash.Dialect(), dash.Style(), true
		case strings.Contains(first, "ksh"):
			return src, ksh.Dialect(), ksh.Style(), true
		case strings.HasSuffix(first, "/sh"), strings.HasSuffix(first, " sh"):
			return src, syntax.Core(), syntax.CoreStyle(), true
		}
		return "", syntax.Dialect{}, syntax.Style{}, false
	case strings.HasPrefix(first, "#compdef"), strings.HasPrefix(first, "#autoload"):
		return src, zsh.Dialect(), zsh.Style(), true
	}
	switch strings.TrimPrefix(filepath.Ext(path), ".") {
	case "sh":
		return src, syntax.Core(), syntax.CoreStyle(), true
	case "bash":
		return src, bash.Dialect(), bash.Style(), true
	case "zsh", "zsh-theme":
		return src, zsh.Dialect(), zsh.Style(), true
	}
	return "", syntax.Dialect{}, syntax.Style{}, false
}

func verify(src string, f *syntax.File, d syntax.Dialect, st syntax.Style) string {
	cs := comments.Recover(src, f)
	out := printer.Format(src, f, cs, st)
	fOut, err := syntax.Parse(out, d)
	if err != nil {
		return "output-unparseable"
	}
	// SameProgram, not two canonical prints compared: printing both through
	// the canonicalizer erases exactly the differences a formatter is most
	// likely to introduce, so two scripts that differ can print alike.
	if _, ok := syntax.SameProgram(f, fOut); !ok {
		return "program-changed"
	}
	if !sameComments(cs, comments.Recover(out, fOut)) {
		return "comments-changed"
	}
	f2, _ := syntax.Parse(out, d)
	if printer.Format(out, f2, comments.Recover(out, f2), st) != out {
		return "not-idempotent"
	}
	return ""
}

func sameComments(a, b []comments.Comment) bool {
	if len(a) != len(b) {
		return false
	}
	at := make([]string, len(a))
	bt := make([]string, len(b))
	for i := range a {
		at[i], bt[i] = a[i].Text, b[i].Text
	}
	sort.Strings(at)
	sort.Strings(bt)
	for i := range at {
		if at[i] != bt[i] {
			return false
		}
	}
	return true
}
