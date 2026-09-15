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
func (r *Runner) reportedPath(name, resolved string) string {
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
