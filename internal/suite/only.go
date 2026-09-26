// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Selectable is every file name a run over these columns could match, rooted
// at dir.
//
// The shape is the same for a fetched suite and for ours: a column names the
// directories it claims relative to a root, so the fetched path passes the
// unpacked tree and the native path passes share/suite. One function rather
// than two because the two callers of [CheckOnly] have to agree about what a
// name is, and a second copy is how they would come to differ.
//
// A directory that cannot be read contributes nothing rather than an error.
// That is safe in the one direction it matters: a name set that is short can
// only make [CheckOnly] refuse a selection it would otherwise have accepted,
// never accept one it would otherwise have refused — and a claimed directory
// that is not there is already a loud failure of its own, in [Suite.Missing]
// for our columns and in plan for a fetched one.
func Selectable(dir string, cols ...Suite) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range cols {
		dirs := s.Dirs
		if len(dirs) == 0 {
			dirs = []string{s.TestDir}
		}
		for _, d := range dirs {
			names, err := Files(filepath.Join(dir, filepath.FromSlash(d)), s.Ext)
			if err != nil {
				continue
			}
			for _, n := range names {
				if !seen[n] {
					seen[n] = true
					out = append(out, n)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// OnlyError is the -only names that no file in the run carries.
//
// It is an error and not a warning, and that is the whole of #4439 and
// #4671. A selector matching nothing runs nothing, and a run of nothing
// prints 0/0 strict, 0 differing lines and a closing line saying every case
// agreed — which is, to the letter, what a file that reached parity prints.
// The figure that gets copied into an issue is the same figure either way.
// AGENTS.md states the rule this is an instance of: "it ran and passed" and
// "it ran and had nothing to look at" are the same line of output.
//
// -column already refuses a name it does not have. This makes the two
// selectors behave alike.
type OnlyError struct {
	// Names are the unmatched selectors, sorted.
	Names []string
	// Meant is, for the ones where a file is an obvious typo away, that
	// file. The two typos worth naming are the ones that were actually made:
	// a path where a base name was wanted (#4439) and the extension left off
	// (#4671).
	Meant map[string]string
	// Files is how many files the selection was held against, so a reader
	// can tell "you typed it wrong" from "this column is empty".
	Files int
}

func (e *OnlyError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "-only: no file is named %s", strings.Join(quoteAll(e.Names), ", "))
	fmt.Fprintf(&b, " (the selection was held against %d files)", e.Files)
	for _, n := range e.Names {
		if m := e.Meant[n]; m != "" {
			fmt.Fprintf(&b, "\n  %q: did you mean %q?", n, m)
		}
	}
	return b.String()
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = fmt.Sprintf("%q", n)
	}
	return out
}

// CheckOnly refuses a selection where some name matches no file in have.
//
// Every name, not the whole list: a comma-separated selection can be partly
// right, and a run scoped to the half that matched is a narrower measurement
// than the one that was asked for, reported as if it were that one.
//
// It answers from a directory listing, so it costs no shell and it refuses
// before the sweep rather than after it.
func CheckOnly(only map[string]bool, have []string) error {
	if len(only) == 0 {
		return nil
	}
	present := map[string]bool{}
	for _, n := range have {
		present[n] = true
	}
	var missing []string
	for n := range only {
		if !present[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	err := &OnlyError{Names: missing, Files: len(have)}
	for _, n := range missing {
		if m := meant(n, have); m != "" {
			if err.Meant == nil {
				err.Meant = map[string]string{}
			}
			err.Meant[n] = m
		}
	}
	return err
}

// meant is the file an unmatched name was probably reaching for.
//
// Two typos, both measured rather than imagined: `-only bash/shell-options.tests`
// where the base name was wanted, and `-only E01options` with the extension
// left off. Both are one keystroke from a real run and neither said so.
func meant(name string, have []string) string {
	base := path.Base(filepath.ToSlash(name))
	for _, h := range have {
		if h == base {
			return h
		}
	}
	for _, h := range have {
		if strings.TrimSuffix(h, filepath.Ext(h)) == base {
			return h
		}
	}
	return ""
}
