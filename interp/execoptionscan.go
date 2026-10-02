// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// execOptionWordsAhead says the words behind this modifier are `exec`'s own
// options, to be read before any of them is matched against the filesystem.
// See Semantics.ExecOptionsAreReadBeforeGlobbing.
func (r *Runner) execOptionWordsAhead(field string) bool {
	return globUnescape(field) == "exec" && r.sem().ExecOptionsAreReadBeforeGlobbing == Yes
}

// execOptionScan is the precommand scan's reading of `exec`'s option words,
// which can run across words: `-a` takes the next one.
type execOptionScan struct {
	// reading says the scan is still among the options.
	reading bool
	// nameNext says the word before was a bundle ending in `a`, so this one
	// is the name and not an option.
	nameNext bool
}

// take answers how many of fields are exec's options or the operand of its
// `-a`, ending the reading at the first that is neither or after a `--`.
//
// The letters are not checked here: Runner.execOptions does that once the
// words are in, and a letter it refuses is refused whichever way the word
// was read. Only the shape matters to the scan — a bundle ending in `a` takes
// the word behind it, one with text after the `a` has its name inside it.
func (s *execOptionScan) take(fields []string) int {
	for i, f := range fields {
		if s.nameNext {
			s.nameNext = false
			continue
		}
		f = globUnescape(f)
		if !strings.HasPrefix(f, "-") || f == "-" {
			s.reading = false
			return i
		}
		if f == "--" {
			s.reading = false
			return i + 1
		}
		s.nameNext = strings.IndexByte(f, 'a') == len(f)-1
	}
	return len(fields)
}
