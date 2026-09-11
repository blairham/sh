// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"maps"
	"strings"
)

// A here-document body feeding a command the shell runs as a process of its
// own is expanded *in that process*, and this file is what makes that true
// here.
//
// Measured 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME,
// ZDOTDIR and HISTFILE, over a script file, against bash 5.3.15, ksh93u+,
// zsh 5.9.2 and dash. `n=1` and a body of `$(( n++ ))`:
//
//	                                   bash  ksh93  zsh  dash
//	cat <<END          (external)         1      1    1     2
//	command -p cat <<END                  1      1    1     -
//	nosuchcmd <<END    (exec fails)       1      1    1     -
//	cat <<END &        (background)       1      1    1     -
//	cat <<END | cat    (pipeline)         1      1    1     -
//	read x <<END       (builtin)          2      2    2     2
//	: <<END            (builtin)          2      2    2     2
//	exec 3<<END        (builtin)          2      2    2     -
//	f <<END            (function)         2      2    2     -
//	{ cat; } <<END     (group)            2      2    2     -
//	while read x; …done <<END             2      2    2     -
//	eval 'cat' <<END                      2      2    2     -
//	. /dev/null <<END                     2      2    2     -
//
// dash has no `++`, so its column is `${u:=zz}` instead — and dash is the
// one shell where the write escapes an external command's here-document as
// well, which is why the confinement is an axis and not a core rule.
//
// The line the table draws is not "a here-document" and not "a redirection":
// it is whether the shell runs the command itself. Everything the shell runs
// itself — a builtin, a function, a compound command, `exec`, `eval`, `.` —
// leaves the write behind in all four columns, because there is no other
// process for it to land in.
//
// Two things follow from putting the expansion in the child, and they are
// separate questions:
//
//   - The **value** is unaffected. The increment happens, so a body of
//     `[$(( n++ ))][$(( n++ ))]` is `[1][2]` in bash, ksh93 and zsh alike —
//     the write is visible to the rest of the body and gone afterwards. That
//     is why this saves and puts back rather than refusing the write: a
//     refusal would have produced `[1][1]`, which is nobody's answer.
//
//   - A **failed** expansion costs the command rather than the shell, and
//     the command does not run. That half is unanimous, dash included:
//     `set -u` with an unset name in the body of `cat <<END` writes the
//     diagnostic, `cat` never runs, the status is the dialect's fatal one,
//     and the next command in the script does. Here the body's value was
//     handed to `cat` anyway and then the whole script was abandoned — one
//     wrong answer on top of the other. See giveUpTheCommand.

// expansionTables is the state an expansion can write, held so a body
// expanded on another process's behalf can be undone.
//
// The list is the tables the assignment doors touch — setVarAs for a scalar,
// storeArray and setArrayElem for an element, setAssocElem for a key — and
// the three side tables setVarAs keeps beside Vars. Not a whole clone(),
// because a clone copies streams, descriptors, jobs and traps that a
// here-document body has no business rearranging, and because a list this
// short is one a reader can check against the doors.
//
// What it deliberately does not cover is a *produced* parameter: a name a
// front end registered a writer for is that front end's state, and putting
// it back is not this package's to do. In a real shell such a write lands in
// the child too, so the gap is a gap; it is recorded here rather than
// pretended away.
type expansionTables struct {
	vars          map[string]string
	arrays        map[string]Array
	assoc         map[string]AssocArray
	exported      map[string]bool
	removed       map[string]bool
	declaredEmpty map[string]bool
	assigned      map[string]string
}

// saveExpansionTables takes the copy. Every map is cloned one level deep, and
// the two array tables a second level, because their values are maps too —
// cloning only the outer one would have shared the very element an element
// assignment writes.
func (r *Runner) saveExpansionTables() expansionTables {
	return expansionTables{
		vars:          maps.Clone(r.Vars),
		arrays:        cloneArrays(r.Arrays),
		assoc:         cloneAssoc(r.AssocArrays),
		exported:      maps.Clone(r.exported),
		removed:       maps.Clone(r.removed),
		declaredEmpty: maps.Clone(r.declaredEmpty),
		assigned:      maps.Clone(r.assigned),
	}
}

// restoreExpansionTables puts the copy back.
func (r *Runner) restoreExpansionTables(t expansionTables) {
	r.Vars, r.Arrays, r.AssocArrays = t.vars, t.arrays, t.assoc
	r.exported, r.removed, r.declaredEmpty, r.assigned = t.exported, t.removed, t.declaredEmpty, t.assigned
}

// wroteSince reports whether anything in the tables has changed.
//
// The axis below is asked only when it has, which keeps the question at the
// disagreement: a here-document body with no side effect is the overwhelming
// case and every shell agrees about it, so a shell whose dialect has not
// answered must still be able to run one.
func (r *Runner) wroteSince(t expansionTables) bool {
	return !maps.Equal(t.vars, r.Vars) ||
		!arraysEqual(t.arrays, r.Arrays) ||
		!assocEqual(t.assoc, r.AssocArrays) ||
		!maps.Equal(t.exported, r.exported) ||
		!maps.Equal(t.removed, r.removed) ||
		!maps.Equal(t.declaredEmpty, r.declaredEmpty) ||
		!maps.Equal(t.assigned, r.assigned)
}

func cloneArrays(in map[string]Array) map[string]Array {
	if in == nil {
		return nil
	}
	out := make(map[string]Array, len(in))
	for k, v := range in {
		out[k] = maps.Clone(v)
	}
	return out
}

func cloneAssoc(in map[string]AssocArray) map[string]AssocArray {
	if in == nil {
		return nil
	}
	out := make(map[string]AssocArray, len(in))
	for k, v := range in {
		out[k] = maps.Clone(v)
	}
	return out
}

func arraysEqual(a, b map[string]Array) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !maps.Equal(v, w) {
			return false
		}
	}
	return true
}

func assocEqual(a, b map[string]AssocArray) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !maps.Equal(v, w) {
			return false
		}
	}
	return true
}

// confineToTheProcess expands a here-document body the way the process the
// redirection is for would, and reports what to do with the result.
//
// The wrapper rather than a flag inside the expander, because the expander
// must not know: the body expands exactly as it always did, and the whole of
// this is what happens to the writes it left behind.
func (r *Runner) confineToTheProcess(expand func() string) string {
	before := r.saveExpansionTables()
	body := expand()
	if r.wroteSince(before) {
		if r.ask(r.sem().HeredocExpandsInTheCommandsProcess,
			"a here-document body's side effect reaching the shell that fed it") {
			r.restoreExpansionTables(before)
		} else if r.unspecified {
			// Nobody answered, so there is no telling whose the write is.
			// Reporting the refusal and handing the body to the program
			// anyway is the silent wrong answer this whole structure exists
			// to avoid: the status the refusal left stands, and the command
			// does not run.
			r.redirErr = true
			return body
		}
	}
	r.giveUpTheCommand()
	return body
}

// giveUpTheCommand is the abandonment boundary for a redirection that could
// not be expanded — a here-document body, or the target of a redirection:
// the unit given up is the *command*, because the error happened in a
// process that is not this shell.
//
// The fourth site of the mechanism `.`, `eval`, a startup file and an
// interactive prompt already use — pendingFileError and takeFileError — and
// the axis those sites split on is not asked here, because at this boundary
// there is nothing to ask: measured, `set -u` with an unset name in the body
// of `cat <<END` leaves bash, ksh93, zsh and dash all reporting a failure,
// not running `cat`, and running the next command in the script.
//
// `exit` is still not caught, for the reason the other sites do not catch it:
// `cat <<END` with `$(exit 3)` in its body is a substitution's own exit and
// has never reached here, and a request to stop that did would be a request
// to stop.
func (r *Runner) giveUpTheCommand() {
	if r.expandErr && r.ctl == controlNone {
		// A failure that reported itself and set no control flow — a bad
		// substitution in the dialects that word it that way. It still has
		// to cost the command, and fatalQuiet is what gives it a status.
		r.fatalQuiet()
	}
	if !r.pendingFileError() {
		return
	}
	r.takeFileError()
	// The status fatalQuiet or whoever failed already set stands; what this
	// adds is that the command does not run. redirErr is the flag both
	// simple and withRedirs already read for "the redirections did not come
	// out", which is exactly what happened.
	r.redirErr = true
	r.expandErr = false
}

// commandRunsInThisShell reports whether the shell will run this command
// itself rather than as a process of its own.
//
// The question a real shell answers by forking, asked here because this shell
// does not fork: a builtin, a function and a command the shell refuses to
// have all stay in this process, and everything else becomes one of its own.
// Which side a command falls on decides where its here-document body is
// expanded, and nothing else — see the table at the top of this file.
//
// `command` is unwrapped because it is a builtin that runs something else,
// and the shells follow what it reaches: `command -p cat <<END` confines the
// write in bash, ksh93 and zsh exactly as a bare `cat` does. Functions are
// not consulted past it, because bypassing them is what `command` is for.
//
// `command echo` is the one shape in this file the panel splits on and this
// does not ask about: bash and ksh93 leave the write behind, zsh confines it.
// The write escaping is what this shell already did, so the two-to-one is the
// answer it keeps; a field for it would be a question about `command` rather
// than about here-documents, and no measurement here makes it one.
func (r *Runner) commandRunsInThisShell(argv []string) bool {
	if len(argv) == 0 {
		// Assignments and redirections with no command name. There is no
		// other process, so this one is it.
		return true
	}
	name := argv[0]
	if name == "command" {
		return r.commandBuiltinRunsInThisShell(argv[1:])
	}
	if _, ok := r.funcs[name]; ok {
		return true
	}
	if _, ok := r.lookupBuiltin(name); ok {
		return true
	}
	// A builtin this shell will not run as a program never becomes a process
	// either: it is refused here, in this process, before anything is
	// started. See reserved.go.
	return r.reservedBuiltin(name)
}

// commandBuiltinRunsInThisShell is the same question for what `command` will
// reach, given its arguments.
//
// The option words are walked with the same reader biCommand uses, and an
// unknown one ends the walk rather than being judged: whether `command -q ls`
// is a refusal or a command named `-q` is an axis, and either way the answer
// to *this* question is the same, because neither a refusal nor a name
// beginning with a dash is a builtin.
func (r *Runner) commandBuiltinRunsInThisShell(args []string) bool {
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if a == "--" {
			args = args[1:]
			break
		}
		letters, ok := commandOptionLetters(a)
		if !ok {
			break
		}
		if strings.ContainsAny(letters, "vV") {
			// `command -v name` and `command -V name` run nothing at all.
			// They answer a question, in this shell.
			return true
		}
		args = args[1:]
	}
	if len(args) == 0 {
		return true
	}
	_, isBuiltin := r.lookupBuiltin(args[0])
	return isBuiltin || r.reservedBuiltin(args[0])
}
