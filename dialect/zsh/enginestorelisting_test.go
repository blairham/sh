// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// This shell keeps its own state in the parameter tables, under names
// beginning `.zsh.`, because that is what a subshell copies. The reference
// keeps the same facts where no script can see them.
//
// A *read* of one was already shut — `${.zsh.zmodload}` is `bad substitution`
// in both shells, a leading `.` being no identifier anywhere — and a
// **listing** was not. Measured 2026-09-28 against `/opt/homebrew/bin/zsh` —
// zsh 5.9.2 (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go
// executable* for it — `-f` from a script file under `env -i
// PATH=/usr/bin:/bin TERM=dumb` with a scratch `HOME`, after `zmodload
// zsh/langinfo`:
//
//	zsh 5.9.2   (nothing)
//	ours        array .zsh.zmodload=( zsh/main zsh/langinfo )
//
// The row was found by a test grepping a bare `typeset` for `*watch*` and
// matching the *value* of `.zsh.zmodload` rather than a name — so it is worth
// saying that the value moves as modules load, which is a listing that
// differs from itself for a reason nothing in the script explains.
//
// **This test is the general form rather than the one name.** The script
// below loads modules, fills the `zstyle` table and schedules an event, so
// six separate stores are non-empty when it walks; the assertion is that no
// name a listing writes begins with a dot, whichever store put it there
// (#5014).
func TestNoEngineStoreReachesAListing(t *testing.T) {
	// One listing per route, each writing the names it reached. Every route
	// here wrote at least one `.zsh.` name before the fix, which is what
	// makes a clean run evidence rather than an instrument with nothing in
	// front of it.
	const src = `zmodload zsh/parameter
zmodload zsh/langinfo
zmodload zsh/datetime
zstyle ':x:*' foo bar
sched +99 :
for route in 'typeset' 'typeset -p' 'typeset +' 'typeset -a' 'typeset -g' \
             'typeset -m ".*"' 'set' 'declare' 'print -l ${(k)parameters}' \
             'export -p' 'readonly -p'; do
  eval "$route" 2>/dev/null | while IFS= read -r line; do
    case $line in
      (*[[:space:]].*|.*) print -r -- "$route: $line" ;;
    esac
  done
done
print DONE
`
	script := zshScriptFileAt(t, src)
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
	if code != 0 {
		t.Fatalf("the walk itself answered %d (said %q), so it listed nothing", code, errs.String())
	}
	if !strings.HasSuffix(out.String(), "DONE\n") {
		t.Fatalf("the walk did not reach its end: %q", out.String())
	}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "DONE\n"), "\n") {
		if line == "" {
			continue
		}
		t.Errorf("a listing wrote a name a script cannot write: %s", line)
	}
}

// And the control, which is what says the walk above can see anything at all:
// the same routes over an ordinary name write rows for it. Without this, a
// walk whose `eval` had silently listed nothing would pass.
func TestTheSameListingsDoReachAnOrdinaryName(t *testing.T) {
	const src = `zmodload zsh/parameter
typeset -g dotless=v
for route in 'typeset' 'typeset -p' 'typeset +' 'typeset -g' 'set' 'declare' \
             'print -l ${(k)parameters}'; do
  n=$(eval "$route" 2>/dev/null | grep -c dotless)
  print -r -- "$route=$n"
done
`
	script := zshScriptFileAt(t, src)
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
	if code != 0 {
		t.Fatalf("answered %d (said %q)", code, errs.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if strings.HasSuffix(line, "=0") {
			t.Errorf("%s — this route lists nothing, so it proves nothing about the dotted names", line)
		}
	}
}

// TestUnsetByPatternDoesNotReachTheEngineState is the route with teeth, and
// the reason the rule is not only about listings: `unset -m` walks the
// parameter tables by pattern, and a pattern a script writes for its own
// names reached state no script put there.
//
// Measured 2026-09-28 the same way — `zmodload zsh/langinfo; unset -m '.zsh*';
// zmodload -e zsh/langinfo` is **0** in zsh 5.9.2, the module still loaded,
// and was **1** here: the pattern had emptied the loaded set. A build with
// the guard taken back out answers 1 again, so the row is this guard's and
// not the prefix's spelling.
func TestUnsetByPatternDoesNotReachTheEngineState(t *testing.T) {
	const src = "zmodload zsh/langinfo\n" +
		"unset -m '.zsh*'\n" +
		"zmodload -e zsh/langinfo; print \"e=$?\"\n" +
		"print -r -- \"${langinfo[CODESET]:+has-langinfo}\"\n"
	script := zshScriptFileAt(t, src)
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
	if out.String() != "e=0\nhas-langinfo\n" || code != 0 {
		t.Errorf("wrote %q at %d (said %q), want the module still loaded",
			out.String(), code, errs.String())
	}
}

// The read was already shut and stays shut: a name under the prefix is not a
// parameter a script can ask for, in either shell.
func TestAnEngineStoreIsStillNoNameToRead(t *testing.T) {
	script := zshScriptFileAt(t, "zmodload zsh/langinfo\nprint -r -- \"${.zsh.zmodload}\"\n")
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-f", script})
	if code == 0 || !strings.Contains(errs.String(), "bad substitution") {
		t.Errorf("answered %d saying %q, want the reference's `bad substitution`", code, errs.String())
	}
	if out.String() != "" {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}
