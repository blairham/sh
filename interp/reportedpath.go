// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"path/filepath"
	"strings"
)

// reportedPath is what a builtin asked *where* a command is writes for its
// operand, given the path the search resolved.
//
// One function for every such builtin — `command -v`, `command -V`, `type`
// and its letters, and a dialect's own `whence` — because the panel answers
// them all the same way within a shell and a second copy of the rule is how
// two of them would come to disagree. See
// Semantics.APathnameOperandIsReportedAbsolute for the measurements.
//
// A name with no slash in it was found by searching PATH, and the resolved
// path is the answer in every column: that is the part a script cannot work
// out for itself and it is not this axis. A name *with* a slash was never
// searched for, and what is written for it splits the panel.
//
// Neither reading cleans, so the resolved path is not what the joining shell
// writes either — filepath.Abs tidies a `./` away and ksh93 keeps it. The
// join is spelled out here for that reason: it is a prefix join, and an
// absolute operand has nothing to join to.
// The word is written back the way the dialect writes one it could not
// otherwise write bare, which is Runner.NameReportWord and is empty in every
// column but one. Here rather than at each print, because this is the one
// place every such builtin passes through and the sentence forms and the
// bare-path forms have to agree: measured 2026-09-18 on ksh93u+ 2012-08-01,
// `whence 'a b'` is `'/…/a b'` and `whence -v 'a b'` is
// `'a b' is a tracked alias for '/…/a b'`, so the path is quoted whether or
// not there is a sentence around it — and `whence zz` with a blank in the
// *directory* quotes the path while leaving the plain name alone, which is
// what says the two words are written back one at a time (#3678).
func (r *Runner) reportedPath(name, resolved string) string {
	return r.NameReportWord(r.resolvedPathFor(name, resolved))
}

// resolvedPathFor is which of the two paths is the answer, before it is
// written back.
func (r *Runner) resolvedPathFor(name, resolved string) string {
	if !strings.ContainsRune(name, '/') {
		return resolved
	}
	if !r.ask(r.sem().APathnameOperandIsReportedAbsolute,
		"a pathname operand written back as an absolute path by a builtin asked where a command is") {
		return name
	}
	if filepath.IsAbs(name) || r.Dir == "" {
		return name
	}
	return r.Dir + "/" + name
}
