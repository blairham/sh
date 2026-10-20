// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package shtest runs a dialect's case files: shell snippets with the output
// and status they must produce, kept as data beside the Go tests rather than
// as Go.
//
// A case file is txtar-shaped. Everything before the first section is prose —
// what the case pins and where it was measured — and is not read. Then:
//
//	-- config --           optional, once, before the first script:
//	                         name: sh                  $0 for the run
//	                         env: PATH=/usr/bin:/bin   replaces the scratch PATH
//	-- file NAME --        written into the case's scratch directory
//	-- script [LABEL] --   starts a case; the snippet itself
//	-- out --              the combined stdout and stderr, exactly
//	-- contains --         each non-empty line must appear in the output
//	-- lacks --            no non-empty line may appear in the output
//	-- suffix --           the output must end with this
//	-- status --           an integer, or "nonzero"
//
// A section's text always ends in a newline. Add `noeol` to the name —
// `-- out noeol --` — to drop it, for output that has none.
//
// The default run is the one nearly every dialect helper performs: the
// dialect's preset, a fresh scratch directory as both the working directory
// and PATH, stdout and stderr on one stream. A case with no expectation is an
// error, not a pass: an empty check reads exactly like a passing one.
package shtest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Case is one script and what it must produce.
type Case struct {
	Label  string
	Script string
	Out    *string
	Suffix *string
	Has    []string
	Lacks  []string
	Status string // "" is unchecked
}

// File is one parsed case file.
type File struct {
	Name  string // $0 override
	Env   []string
	Files map[string]string
	Cases []Case
}

// Run runs every *.txtar under dir as a subtest named for the file.
func Run(t *testing.T, p dialecttest.Preset, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no case files under %s", dir)
	}
	sort.Strings(paths)
	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".txtar"), func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(string(data))
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			for i, c := range f.Cases {
				label := c.Label
				if label == "" {
					label = strconv.Itoa(i + 1)
				}
				t.Run(label, func(t *testing.T) {
					t.Parallel()
					runCase(t, p, f, c)
				})
			}
		})
	}
}

func runCase(t *testing.T, p dialecttest.Preset, f *File, c Case) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range f.Files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := dialecttest.Base{Name: f.Name, Dir: dir}
	if f.Env != nil {
		b.Env = f.Env
	} else {
		b.Vars = map[string]string{"PATH": dir}
	}
	out, st, err := p.Combined(t, b, c.Script)
	if err != nil {
		t.Fatalf("run %q: %v", c.Script, err)
	}
	fail := func(what string, want any) {
		t.Errorf("%s\n got %q (status %d)\nwant %s %q", c.Script, out, st, what, want)
	}
	if c.Out != nil && out != *c.Out {
		fail("exactly", *c.Out)
	}
	if c.Suffix != nil && !strings.HasSuffix(out, *c.Suffix) {
		fail("a suffix", *c.Suffix)
	}
	for _, s := range c.Has {
		if !strings.Contains(out, s) {
			fail("it to contain", s)
		}
	}
	for _, s := range c.Lacks {
		if strings.Contains(out, s) {
			fail("it to lack", s)
		}
	}
	switch c.Status {
	case "":
	case "nonzero":
		if st == 0 {
			t.Errorf("%s\n status 0, want nonzero (output %q)", c.Script, out)
		}
	default:
		if want, _ := strconv.Atoi(c.Status); st != want {
			t.Errorf("%s\n status %d, want %d (output %q)", c.Script, st, want, out)
		}
	}
}

// Parse reads one case file.
func Parse(src string) (*File, error) {
	f := &File{Files: map[string]string{}}
	var cur *Case
	for _, s := range sections(src) {
		body := s.body
		if s.noeol {
			body = strings.TrimSuffix(body, "\n")
		}
		switch {
		case s.kind == "config":
			if cur != nil {
				return nil, fmt.Errorf("config after a script")
			}
			for _, line := range lines(body) {
				k, v, ok := strings.Cut(line, ":")
				v = strings.TrimSpace(v)
				switch {
				case ok && k == "name":
					f.Name = v
				case ok && k == "env":
					f.Env = append(f.Env, v)
				default:
					return nil, fmt.Errorf("unknown config line %q", line)
				}
			}
		case s.kind == "file":
			if s.arg == "" || strings.ContainsRune(s.arg, '/') {
				return nil, fmt.Errorf("file section needs a plain name, got %q", s.arg)
			}
			f.Files[s.arg] = body
		case s.kind == "script":
			f.Cases = append(f.Cases, Case{Label: s.arg, Script: body})
			cur = &f.Cases[len(f.Cases)-1]
		case cur == nil:
			return nil, fmt.Errorf("%q before any script", s.kind)
		case s.kind == "out":
			cur.Out = &body
		case s.kind == "suffix":
			cur.Suffix = &body
		case s.kind == "contains":
			cur.Has = append(cur.Has, lines(body)...)
		case s.kind == "lacks":
			cur.Lacks = append(cur.Lacks, lines(body)...)
		case s.kind == "status":
			v := strings.TrimSpace(body)
			if _, err := strconv.Atoi(v); err != nil && v != "nonzero" {
				return nil, fmt.Errorf("status %q is neither a number nor nonzero", v)
			}
			cur.Status = v
		default:
			return nil, fmt.Errorf("unknown section %q", s.kind)
		}
	}
	if len(f.Cases) == 0 {
		return nil, fmt.Errorf("no script sections")
	}
	for _, c := range f.Cases {
		if c.Out == nil && c.Suffix == nil && c.Has == nil && c.Lacks == nil && c.Status == "" {
			return nil, fmt.Errorf("script %q checks nothing", c.Script)
		}
	}
	return f, nil
}

type section struct {
	kind, arg string
	noeol     bool
	body      string
}

// sections splits txtar: a header line is `-- words --` on a line of its own.
func sections(src string) []section {
	var out []section
	var cur *section
	var body strings.Builder
	flush := func() {
		if cur != nil {
			cur.body = body.String()
			out = append(out, *cur)
		}
		body.Reset()
	}
	for _, line := range strings.SplitAfter(src, "\n") {
		trimmed := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(trimmed, "-- ") && strings.HasSuffix(trimmed, " --") && len(trimmed) > 6 {
			flush()
			words := strings.Fields(trimmed[3 : len(trimmed)-3])
			s := section{kind: words[0]}
			for _, w := range words[1:] {
				if w == "noeol" {
					s.noeol = true
				} else {
					s.arg = strings.TrimSpace(s.arg + " " + w)
				}
			}
			cur = &s
			continue
		}
		if cur != nil {
			body.WriteString(line)
		}
	}
	flush()
	return out
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
