// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// A builtin names itself between the script and the line: `/s.sh: export:
// line 1: illegal option -q` (#2761).
//
// The second shell in the panel to do it, after zsh — which writes
// `zsh:export:1:`, the same segment with the punctuation its own location
// style uses. So this is interp.Diagnostics.NamesBuiltinInLocation, a value
// rather than a mechanism, and the separator follows the style.
//
// Measured 2026-09-14, BusyBox v1.37.0 in the pinned alpine image, each line
// alone in a script file under `env -i`. Twenty builtins were asked and every
// one names itself; these are the shapes that differ from each other rather
// than twenty copies of one row.
func TestABuiltinNamesItselfInTheLocation(t *testing.T) {
	for _, tc := range []struct{ name, src, want, why string }{
		{
			"an option it does not take",
			"export -q\n", ": export: line 1: illegal option -q",
			"the row the issue was filed on",
		},
		{
			"a count it will not read",
			"shift -1\n", ": shift: line 1: Illegal number: -1",
			"a different builtin and a different wording, so the segment is not part of one sentence",
		},
		{
			"a name it will not take",
			"readonly 1bad=2\n", ": readonly: line 1: 1bad: bad variable name",
			"a declaration builtin",
		},
		{
			"the dot is spelled with its own name",
			". /nonexistent/file\n",
			": .: line 1: can't open '/nonexistent/file': No such file or directory",
			"the one whose name is punctuation, and it is written as `.` rather than skipped",
		},
		{
			"a status operand it will not read",
			"exit abc\n", ": exit: line 1: Illegal number: abc",
			"a builtin that was on its way out still names itself",
		},
		{
			"a directory that is not there",
			"cd /nonexistent/dir\n",
			": cd: line 1: can't cd to /nonexistent/dir: No such file or directory",
			"the message already opened with the name in the other dialects, so the segment must not double it",
		},
		{
			"a condition that names no signal",
			"trap '' NOSUCHSIG\n",
			": trap: line 1: NOSUCHSIG: invalid signal specification",
			"the wording as well as the location: this shell says `invalid signal " +
				"specification` where dash says `bad trap`, which is why the issue calls " +
				"this row two facts",
		},
		{
			"a readonly name unset refuses to remove",
			"readonly r=1\nunset r\n", ": unset: line 2: r: is read only",
			"named here where zsh — the other shell that names builtins — writes " +
				"`zsh:2: read-only variable: r` with no segment at all",
		},
		{
			"a command exec cannot find",
			"exec /nonexistent/x\n", ": exec: line 1: /nonexistent/x: not found",
			"named here where zsh calls the same failure the shell's own and reports " +
				"exactly what a bare command word reports",
		},
		{
			"a builtin outside a function",
			"local x=1\n", ": local: line 1: not in a function",
			"a refusal whose sentence names nothing, so the segment is the only place the builtin appears",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runIn(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q — %s", out, tc.want, tc.why)
			}
		})
	}
}

// What stays bare, and it is as consistent as what does not: these are the
// shell's own failures rather than a builtin's. Measured in the same run.
//
// The last row is the sharpest, because a builtin *is* involved: a
// redirection opened for one keeps the shell's own location and never names
// the builtin — see interp.Diagnostics.BuiltinLocationIsTheSpeakersOnly,
// which is the half of this that only the `-c` route can show.
func TestTheShellsOwnFailuresNameNoBuiltin(t *testing.T) {
	for _, tc := range []struct{ name, src, absent, why string }{
		{"a command that was not found", "nosuchcommandhere\n", ":", "not a builtin's complaint at all"},
		{"an assignment to a readonly name", "readonly r=1\nr=2\n", "r=2", "the shell stores, not a builtin"},
		{"an unset parameter", "set -u\necho $undefinedvar\n", "echo:", "an expansion failure is the shell's"},
		{"a division by zero", ": $((1/0))\n", ":  $", "arithmetic is the shell's"},
		{"a redirection that would not open", "read x < /nonexistent/f\n", "read:", "opened for a builtin and still not the builtin's"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runIn(t, tc.src)
			if strings.Contains(out, ": "+tc.absent+" line ") {
				t.Errorf("got %q, want no builtin segment — %s", out, tc.why)
			}
		})
	}
}

// A builtin's name takes the slot the borrowed text's name would have had,
// rather than standing beside it.
//
// The same rule dash follows for the name it writes *after* the location
// (#2532), measured here on the placement this shell uses. BusyBox v1.37.0,
// 2026-09-14, with `p.sh` holding `shift -1` on its second line:
//
//	./s.sh does `. ./p.sh`         /s.sh: shift: line 2: Illegal number: -1
//	./e.sh does eval "shift -1"    /e.sh: shift: line 1: Illegal number: -1
//	an unset parameter in p.sh     /s.sh: ./p.sh: line 2: NOPE: parameter not set
//
// so the file keeps the slot for what the shell says and loses it to a
// builtin speaking for itself, and the line is the one inside the borrowed
// text either way.
func TestABuiltinTakesTheBorrowedTextsSlot(t *testing.T) {
	out, _ := runIn(t, "printf 'echo one\\nshift -1\\n' > p.sh\n. ./p.sh\n")
	if !strings.Contains(out, ": shift: line 2: ") {
		t.Errorf("got %q, want the builtin named and the file's own line", out)
	}
	if strings.Contains(out, "p.sh:") {
		t.Errorf("got %q, want the sourced file's name gone: the builtin took its slot", out)
	}
	// And the control, so the row above cannot read as "the name is never
	// written": the same file's own failure keeps it.
	out, _ = runIn(t, "printf 'echo one\\necho $NOPE\\n' > p.sh\nset -u\n. ./p.sh\n")
	if !strings.Contains(out, "./p.sh: line 2: NOPE: parameter not set") {
		t.Errorf("got %q, want the sourced file named for its own failure", out)
	}
}
