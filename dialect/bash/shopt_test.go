// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/dialecttest"
)

// What `shopt` does here is what bash 5.3 was measured doing: the listing
// format, the statuses of every flag combination, and which options change
// which behavior. The snippets are ours; the outputs are the binary's.

func TestShoptQueriesAndFormats(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		// A bare name prints the two-column line and answers by status.
		{`shopt nullglob`, "nullglob            \toff\n", 1},
		{`shopt -s nullglob; shopt nullglob`, "nullglob            \ton\n", 0},
		// -p prints the command that would restore the state, status alike.
		{`shopt -p nullglob`, "shopt -u nullglob\n", 1},
		{`shopt -s nullglob; shopt -p nullglob`, "shopt -s nullglob\n", 0},
		// -q is the status alone.
		{`shopt -q nullglob`, "", 1},
		{`shopt -s nullglob; shopt -q nullglob`, "", 0},
		{`shopt -q`, "", 0},
		// Every queried name must be on for success.
		{`shopt -s nullglob; shopt -q nullglob dotglob`, "", 1},
		// `--` ends the options.
		{`shopt -s -- nullglob; shopt -q nullglob`, "", 0},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

func TestShoptListings(t *testing.T) {
	// The full listing carries every name, two-column; nothing else on the
	// full list needs pinning here because the states are asserted one by
	// one above and below.
	out, st := runBash(t, t.TempDir(), `shopt`)
	if st != 0 || !strings.Contains(out, "nullglob            \toff\n") ||
		!strings.Contains(out, "sourcepath          \ton\n") {
		t.Errorf("shopt listed %q status %d", out, st)
	}
	// `-s` alone lists what is on, `-u` what is off, `-p` reissuably.
	out, st = runBash(t, t.TempDir(), `shopt -s nullglob; shopt -s`)
	if st != 0 || !strings.Contains(out, "nullglob            \ton\n") ||
		strings.Contains(out, "dotglob") {
		t.Errorf("shopt -s listed %q status %d", out, st)
	}
	out, st = runBash(t, t.TempDir(), `shopt -u`)
	if st != 0 || !strings.Contains(out, "nullglob            \toff\n") ||
		strings.Contains(out, "sourcepath") {
		t.Errorf("shopt -u listed %q status %d", out, st)
	}
	out, st = runBash(t, t.TempDir(), `shopt -p nullglob dotglob`)
	if st != 1 || out != "shopt -u nullglob\nshopt -u dotglob\n" {
		t.Errorf("shopt -p listed %q status %d", out, st)
	}
}

func TestShoptRefusals(t *testing.T) {
	for _, tc := range []struct {
		src    string
		said   string
		status int
	}{
		{`shopt -s nosuchopt`, "shopt: nosuchopt: invalid shell option name", 1},
		{`shopt nosuchopt`, "shopt: nosuchopt: invalid shell option name", 1},
		{`shopt -su nullglob`, "cannot set and unset shell options simultaneously", 1},
		{`shopt -z`, "shopt: usage: shopt [-pqsu] [-o] [optname ...]", 2},
		// A name this shell recognizes and cannot move: refused out loud,
		// never accepted quietly.
		{`shopt -s failglob`, "shopt: failglob: not implemented", 1},
		{`shopt -u sourcepath`, "shopt: sourcepath: not implemented", 1},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if st != tc.status || !strings.Contains(out, tc.said) {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.said, tc.status)
		}
	}
	// The states already held are granted: off may be turned off, on on.
	for _, src := range []string{`shopt -u failglob`, `shopt -s sourcepath`} {
		if out, st := runBash(t, t.TempDir(), src); st != 0 || out != "" {
			t.Errorf("%s = %q status %d, want a quiet success", src, out, st)
		}
	}
	// The names it knows are switched even when one it does not sits
	// between them, and the status still reports the failure.
	out, st := runBash(t, t.TempDir(),
		`shopt -s nullglob nosuchopt dotglob; shopt -q nullglob && shopt -q dotglob && echo both`)
	if st != 0 || !strings.Contains(out, "both") || !strings.Contains(out, "nosuchopt") {
		t.Errorf("a mixed set gave %q status %d", out, st)
	}
}

func TestShoptWiresTheGlobOptions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s nullglob; echo zz*zz; echo done`, "\ndone"},
		{`shopt -s nullglob; shopt -u nullglob; echo zz*zz`, "zz*zz"},
		{`shopt -s nocasematch; case A in a) echo hit;; esac`, "hit"},
		{`shopt -s nocaseglob; case A in a) echo hit;; *) echo exact;; esac`, "exact"},
	} {
		out, _ := runBash(t, t.TempDir(), tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A subshell's shopt is its own: the option is inherited in and the change
// never escapes.
func TestShoptStaysInsideASubshell(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `(shopt -s nullglob); echo zz*zz`)
	if got := strings.TrimSpace(out); got != "zz*zz" {
		t.Errorf("a subshell's shopt escaped: %q", got)
	}
}

func TestShoptDashOReachesTheSetOptions(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `shopt -so pipefail; false | true; echo ps=$?`)
	if st != 0 || !strings.Contains(out, "ps=1") {
		t.Errorf("shopt -so pipefail gave %q status %d, want the pipe failing", out, st)
	}
	// `shopt -o` alone is `set -o`'s own listing, which is what bash writes
	// there byte for byte. Asserted against `set -o` in the same shell
	// rather than against a copy of the format, since a copy is the thing
	// that drifts.
	out, st = runBash(t, t.TempDir(), `shopt -o`)
	ref, refSt := runBash(t, t.TempDir(), `set -o`)
	if st != 0 || refSt != 0 || out != ref {
		t.Errorf("shopt -o gave %q status %d, want set -o's %q status %d", out, st, ref, refSt)
	}
	out, st = runBash(t, t.TempDir(), `shopt -o -p`)
	ref, refSt = runBash(t, t.TempDir(), `set +o`)
	if st != 0 || refSt != 0 || out != ref {
		t.Errorf("shopt -o -p gave %q status %d, want set +o's %q status %d", out, st, ref, refSt)
	}
}

// `shopt -o name` reads the `set -o` namespace through this builtin's own
// interface, which is the spelling a script uses to test one option without
// parsing `$SHELLOPTS`. It was refused outright before #1429.
//
// Every case pins the *text* as well as the status. A test that only asked
// `shopt -o -s vi; shopt -o -q vi` would pass against a table that stored
// anything at all, which is the failure mode an option table invites: the
// listing and the status together are what separate a working option from a
// bit-bucket.
func TestShoptDashOReadsOneName(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		// A named row is padded to *shopt's* width, not `set -o`'s — the
		// two listings really are two widths, and both are measured.
		{`shopt -o vi`, "vi                  \toff\n", 1},
		{`shopt -o -s vi; shopt -o vi`, "vi                  \ton\n", 0},
		// -p writes a `set` line rather than a `shopt` one, because
		// `shopt -s vi` would not put it back.
		{`shopt -o -p vi`, "set +o vi\n", 1},
		{`shopt -o -s vi; shopt -o -p vi`, "set -o vi\n", 0},
		// -q is the status alone.
		{`shopt -o -q vi`, "", 1},
		{`shopt -o -s vi; shopt -o -q vi`, "", 0},
		{`shopt -o -q`, "", 0},
		// Every name asked for must be on for success, and each is printed.
		{
			`shopt -o -s vi; shopt -o vi xtrace`,
			"vi                  \ton\nxtrace              \toff\n", 1,
		},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
	// The `-o` namespace is not this builtin's own, and neither is its
	// wording: `cdspell` is a fine `shopt` name and not an option name. `-q`
	// silences the *listing* and not the complaint, measured.
	out, st := runBash(t, t.TempDir(), `shopt -o -q cdspell`)
	if st != 1 || !strings.Contains(out, "shopt: cdspell: invalid option name") {
		t.Errorf("shopt -o -q cdspell gave %q status %d", out, st)
	}
	// A name the `set -o` table does not have is refused by that name, and
	// the sentence is a word shorter than the one a bad `shopt` name gets.
	out, st = runBash(t, t.TempDir(), `shopt -o nosuchopt`)
	if st != 1 || !strings.Contains(out, "shopt: nosuchopt: invalid option name") ||
		strings.Contains(out, "invalid shell option name") {
		t.Errorf("shopt -o nosuchopt gave %q status %d", out, st)
	}
	// It carries on past one, printing the names it does have.
	out, st = runBash(t, t.TempDir(), `shopt -o -s vi; shopt -o vi nosuchopt`)
	if st != 1 || !strings.Contains(out, "vi                  \ton\n") ||
		!strings.Contains(out, "nosuchopt") {
		t.Errorf("a mixed shopt -o gave %q status %d", out, st)
	}
}

// `shopt -o -s` and `-o -u` with nothing named narrow `set -o`'s listing to
// the rows that are on or off. The width is `set -o`'s there, not shopt's,
// because the listing is `set -o`'s — measured, and the reason interp
// exposes the rows rather than only the printed listing.
func TestShoptDashOListsNarrowed(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `shopt -o -s vi; shopt -o -s`)
	if st != 0 || !strings.Contains(out, "vi             \ton\n") ||
		strings.Contains(out, "xtrace") {
		t.Errorf("shopt -o -s listed %q status %d", out, st)
	}
	out, st = runBash(t, t.TempDir(), `shopt -o -u`)
	if st != 0 || !strings.Contains(out, "xtrace         \toff\n") ||
		strings.Contains(out, "braceexpand") {
		t.Errorf("shopt -o -u listed %q status %d", out, st)
	}
	// Every row `set -o` writes is in one of the two halves and in neither
	// twice, which is what makes the narrowing a partition rather than a
	// filter that happens to look right.
	n := func(src string) int {
		out, _ := runBash(t, t.TempDir(), src)
		return strings.Count(out, "\n")
	}
	on, off, all := n(`shopt -o -s`), n(`shopt -o -u`), n(`shopt -o`)
	if all == 0 || on+off != all {
		t.Errorf("shopt -o -s (%d) + -o -u (%d) != shopt -o (%d)", on, off, all)
	}
}

// The three history names #1429 asked about, and the six it left refused.
//
// This is the whole point of the issue: a `shopt` that accepted everything
// would put a startup file in the position of believing a setting took
// effect. So the granted names and the refused ones are asserted together —
// the split is the fix, not the count.
func TestShoptHistoryNamesAreOnAndTheRestStayRefused(t *testing.T) {
	// Granted, because this shell already does what each name asks for: the
	// history writer appends rather than rewriting, a construct typed over
	// several lines is one entry, and that entry keeps its newlines rather
	// than being joined with semicolons.
	for _, name := range []string{"histappend", "cmdhist", "lithist"} {
		if out, st := runBash(t, t.TempDir(), "shopt -s "+name); st != 0 || out != "" {
			t.Errorf("shopt -s %s = %q status %d, want a quiet success", name, out, st)
		}
		// Readable as on, in all three spellings, or the grant is a lie.
		out, st := runBash(t, t.TempDir(), "shopt "+name)
		want := fmt.Sprintf("%-20s\ton\n", name)
		if st != 0 || out != want {
			t.Errorf("shopt %s = %q status %d, want %q status 0", name, out, st, want)
		}
		if out, st := runBash(t, t.TempDir(), "shopt -p "+name); st != 0 || out != "shopt -s "+name+"\n" {
			t.Errorf("shopt -p %s = %q status %d", name, out, st)
		}
		// And it must be in the *on* half of the listing, which is the
		// assertion a bit-bucket cannot satisfy.
		out, _ = runBash(t, t.TempDir(), "shopt -s")
		if !strings.Contains(out, want) {
			t.Errorf("shopt -s listing lacks %q: %q", want, out)
		}
		// Turning one off is a promise this shell cannot keep.
		out, st = runBash(t, t.TempDir(), "shopt -u "+name)
		if st != 1 || !strings.Contains(out, "shopt: "+name+": not implemented") {
			t.Errorf("shopt -u %s = %q status %d, want a refusal", name, out, st)
		}
	}
	// Refused, because nothing here implements them. The list is four names
	// shorter than it was: #1445 built `checkwinsize`, `autocd`,
	// `no_empty_cmd_completion` and `cdspell`, and each of those is asserted
	// by behavior below rather than by its bit. What is left is refused for
	// the reason this test exists — granting one would be exactly the quiet
	// lie #1352 named. `dirspell` is refused on the stronger ground that it
	// could not be reproduced in bash 5.3.15 at all; see shopt.go.
	for _, name := range []string{"dirspell", "checkjobs"} {
		out, st := runBash(t, t.TempDir(), "shopt -s "+name)
		if st != 1 || !strings.Contains(out, "shopt: "+name+": not implemented") {
			t.Errorf("shopt -s %s = %q status %d, want a refusal by name", name, out, st)
		}
		// Refused, and still readable as off rather than as absent.
		want := fmt.Sprintf("%-20s\toff\n", name)
		if out, st := runBash(t, t.TempDir(), "shopt "+name); st != 1 || out != want {
			t.Errorf("shopt %s = %q status %d, want %q status 1", name, out, st, want)
		}
	}
}

// bashShell is the same value cmd/bash assembles, minus the prompt wiring the
// tests never reach.
func bashShell(out, errs *bytes.Buffer) driver.Shell {
	return driver.Shell{
		Name:        "bash",
		Dialect:     bash.Dialect(),
		Semantics:   bash.Semantics(),
		Diagnostics: bash.Diagnostics(),
		Prelude:     bash.Prelude(),
		Register:    bash.Apply,
		Stdout:      out,
		Stderr:      errs,
	}
}

// `shopt -s extglob` reaches the parser, one line late by design: the line
// that sets it is already parsed, so the groups are read from the next line
// on — which is measured, and is why both snippets here put the pattern on a
// line of its own.
func TestShoptExtglobTogglesTheGrammarPerLine(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c",
		"shopt -s extglob\ncase ab in @(ab|cd)) echo hit;; esac\n",
	})
	if code != 0 || strings.TrimSpace(out.String()) != "hit" {
		t.Errorf("extglob on: %q / %q status %d, want hit", out.String(), errs.String(), code)
	}

	out.Reset()
	errs.Reset()
	code = driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c",
		"case ab in @(ab|cd)) echo hit;; esac\n",
	})
	if code == 0 || !strings.Contains(errs.String(), "syntax error") {
		t.Errorf("extglob off: %q / %q status %d, want a syntax error",
			out.String(), errs.String(), code)
	}

	// And off again: the grammar follows the option in both directions.
	out.Reset()
	errs.Reset()
	code = driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c",
		"shopt -s extglob\nshopt -u extglob\necho @(ab)\n",
	})
	if code == 0 || !strings.Contains(errs.String(), "syntax error") {
		t.Errorf("extglob off again: %q / %q status %d, want a syntax error",
			out.String(), errs.String(), code)
	}
}

func TestShoptGlobstarThroughTheBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "e"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "e", "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runBash(t, dir, `shopt -s globstar; echo **/f`)
	if got := strings.TrimSpace(out); got != "d/e/f" {
		t.Errorf("globstar gave %q, want d/e/f", got)
	}
	out, _ = runBash(t, dir, `echo **/f`)
	if got := strings.TrimSpace(out); got != "**/f" {
		t.Errorf("without globstar gave %q, want the literal pattern", got)
	}
	// The option covers `**` with nothing behind it as well, which is the
	// half zsh answers the other way — measured 2026-09-07, `shopt -s
	// globstar; echo d/**` in bash 5.3.15 lists the directory itself and
	// every level under it, and `echo **` in zsh 5.9.2 lists one level.
	// The substrate asks the two separately, so this row is what says
	// `globstar` still answers both of them here (#1339).
	out, _ = runBash(t, dir, `shopt -s globstar; echo d/**`)
	if got := strings.TrimSpace(out); got != "d/ d/e d/e/f" {
		t.Errorf("globstar on a bare ** gave %q, want d/ d/e d/e/f", got)
	}
	out, _ = runBash(t, dir, `echo d/**`)
	if got := strings.TrimSpace(out); got != "d/e" {
		t.Errorf("without globstar a bare ** gave %q, want the one level", got)
	}
}

// `shopt -s expand_aliases` is implemented and not merely recognized: the
// word really expands afterwards, and really stops expanding after `-u`.
//
// It is one of the two kinds of "wired" in this table — the matcher's options
// are the other — and it is the one a corpus row depends on, because an alias
// case cannot be written for bash at all without it (#632).
//
// Measured in bash 5.3.15 and bash 3.2.57, on all three non-interactive
// routes: an alias is a command not found until the option is set, `hit`
// after, and a command not found again after `shopt -u`. Every assertion here
// is on the whole stream, location included, because a diagnostic gaining a
// prefix is exactly the failure a substring check cannot see.
func TestShoptExpandAliasesIsALiveSwitch(t *testing.T) {
	var out, errs bytes.Buffer
	run := func(src string) (string, string, int) {
		out.Reset()
		errs.Reset()
		code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
		return out.String(), errs.String(), code
	}

	// Off to begin with: bash is the shell that does not expand in a script.
	o, e, st := run("alias a='echo hit'\na\n")
	if o != "" || e != "bash: line 2: a: command not found\n" || st != 127 {
		t.Errorf("without the option: out %q err %q status %d, want the command not found at 127", o, e, st)
	}

	// Set, and the very next line expands. One line late by design, the way
	// extglob is: the line that sets it has already been parsed.
	o, e, st = run("shopt -s expand_aliases\nalias a='echo hit'\na\n")
	if o != "hit\n" || e != "" || st != 0 {
		t.Errorf("with the option: out %q err %q status %d, want hit at 0", o, e, st)
	}

	// A switch and not a door: `-u` puts it back.
	o, e, st = run("shopt -s expand_aliases\nalias a='echo hit'\na\nshopt -u expand_aliases\na\n")
	if o != "hit\n" || e != "bash: line 5: a: command not found\n" || st != 127 {
		t.Errorf("after -u: out %q err %q status %d, want one hit then the command not found", o, e, st)
	}

	// And the query reports the live switch rather than a stored constant.
	o, _, st = run("shopt expand_aliases")
	if o != "expand_aliases      \toff\n" || st != 1 {
		t.Errorf("query off: %q status %d", o, st)
	}
	o, _, st = run("shopt -s expand_aliases\nshopt expand_aliases\n")
	if o != "expand_aliases      \ton\n" || st != 0 {
		t.Errorf("query on: %q status %d", o, st)
	}
}

// POSIX mode carries the option with it, which is measured rather than
// reasoned: the standard has aliases expand in a script, `set -o posix` makes
// bash expand with no `shopt` written anywhere, and leaving the mode restores
// what the *route* said rather than what was set before entering it.
//
// The last of those is the surprising one and the reason the runner keeps a
// base as well as a live switch: in bash 5.3, `shopt -s expand_aliases; set -o
// posix; set +o posix; shopt expand_aliases` answers `off`.
func TestShoptExpandAliasesFollowsPosixMode(t *testing.T) {
	var out, errs bytes.Buffer
	run := func(argv ...string) (string, string, int) {
		out.Reset()
		errs.Reset()
		code := driver.MainArgs(bashShell(&out, &errs), argv)
		return out.String(), errs.String(), code
	}

	o, e, st := run("bash", "-c", "set -o posix\nalias a='echo hit'\na\n")
	if o != "hit\n" || e != "" || st != 0 {
		t.Errorf("in posix mode: out %q err %q status %d, want hit at 0", o, e, st)
	}

	o, e, st = run("bash", "-c", "shopt -s expand_aliases\nset -o posix\nset +o posix\nalias a='echo hit'\na\n")
	if o != "" || e != "bash: line 5: a: command not found\n" || st != 127 {
		t.Errorf("after the round trip: out %q err %q status %d, want the option back off", o, e, st)
	}

	// The shell invoked as `sh` is in the mode from the start, so it expands
	// where the same binary called `bash` does not — and `set +o posix`
	// inside it turns the expansion off, both measured.
	o, e, st = run("sh", "-c", "alias a='echo hit'\na\n")
	if o != "hit\n" || e != "" || st != 0 {
		t.Errorf("as sh: out %q err %q status %d, want hit at 0", o, e, st)
	}
	o, e, st = run("sh", "-c", "set +o posix\nalias a='echo hit'\na\n")
	if o != "" || e != "sh: line 3: a: command not found\n" || st != 127 {
		t.Errorf("as sh leaving the mode: out %q err %q status %d, want the command not found", o, e, st)
	}
}

// The bare listing names every option exactly once, and there are 59 of them
// — the same 59 bash 5.3.15 lists, and a superset of bash 3.2.57's 34.
//
// The count is the point rather than a detail. A name held in two of the
// table's three maps is invisible to every other test here — the lookups
// check the wired maps first, so the duplicate is shadowed everywhere except
// in the listing, where it prints twice. That is exactly how `expand_aliases`
// would have been left behind by a half-finished move out of the recorded
// table, and nothing else would have said so.
func TestShoptListsEveryNameExactlyOnce(t *testing.T) {
	var out, errs bytes.Buffer
	if code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", "shopt"}); code != 0 {
		t.Fatalf("bare shopt: status %d, err %q", code, errs.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 59 {
		t.Errorf("bare shopt printed %d lines, want bash 5.3's 59", len(lines))
	}
	seen := map[string]int{}
	prev := ""
	for _, l := range lines {
		padded, _, ok := strings.Cut(l, "\t")
		if !ok {
			t.Fatalf("listing line %q has no name", l)
		}
		name := strings.TrimRight(padded, " ")
		seen[name]++
		if name <= prev {
			t.Errorf("listing is out of order at %q, after %q", name, prev)
		}
		prev = name
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("%s appears %d times in the listing, want once", name, n)
		}
	}
	// And the one this change moved is in it, spelled the way the listing
	// spells everything else.
	out.Reset()
	errs.Reset()
	driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", "shopt -p expand_aliases"})
	if got, want := out.String(), "shopt -u expand_aliases\n"; got != want {
		t.Errorf("shopt -p expand_aliases = %q, want %q", got, want)
	}
}

// `shopt -s checkwinsize` is a real switch now, and the assertion is not that
// the bit moved.
//
// A table that stored any value at all would pass a `shopt -s name; shopt -q
// name` round trip, which is why this reads the state through the seam the
// front end reads it through: interp.Runner.TracksWindowSize is what repl
// consults before it assigns $LINES and $COLUMNS, so a name wired to nothing
// would show here as a switch that never moved.
//
// The default is on, which is bash 5.3.15's own — `shopt checkwinsize` there
// answers `on` with nothing in a startup file — and is the one place this
// dialect is deliberately not bash 3.2.57's, which answers `off`.
func TestCheckwinsizeMovesTheShellsWindowTracking(t *testing.T) {
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
	if !r.TracksWindowSize() {
		t.Error("a fresh bash does not track the window size, but `shopt checkwinsize` is on in bash 5.3.15")
	}
	out, st := runBash(t, t.TempDir(), `shopt checkwinsize`)
	if want := "checkwinsize        \ton\n"; out != want || st != 0 {
		t.Errorf("shopt checkwinsize = %q status %d, want %q status 0", out, st, want)
	}
	// And it moves in both directions, through the same seam.
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{`shopt -u checkwinsize`, false},
		{`shopt -u checkwinsize; shopt -s checkwinsize`, true},
	} {
		r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
		f := preset.Parse(t, tc.src)
		if _, err := r.Run(context.Background(), f); err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if got := r.TracksWindowSize(); got != tc.want {
			t.Errorf("%s: TracksWindowSize() = %v, want %v", tc.src, got, tc.want)
		}
	}
}

// `shopt -s autocd` reads a bare directory name as a `cd`, and the test is the
// directory the shell ends up in rather than the bit.
//
// Three claims, and the middle one is the reason a corpus row could not carry
// this: the name is interactive-only in bash as well, so the same snippet has
// to move an interactive shell and *not* move a script.
func TestAutocdReadsADirectoryNameAsACd(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	interactive := dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": dir}, Interactive: true}
	out, st, err := preset.Combined(t, interactive, `shopt -s autocd; sub; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	// The `cd -- sub` bash writes before it moves, then where it moved to.
	if want := "cd -- sub\n" + filepath.Join(dir, "sub") + "\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q status 0", out, st, want)
	}
	// Off, the same word is a command that is not there — which is also what
	// the shell said before this option existed.
	out, st, err = preset.Combined(t, interactive, `sub; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sub: command not found") || !strings.HasSuffix(out, dir+"\n") || st != 0 {
		t.Errorf("without the option: out %q status %d, want a command-not-found and no move", out, st)
	}
	// And a script does not get it even with the option on, which is
	// measured: `bash -c 'shopt -s autocd; sub'` is `command not found` at
	// 127 in bash 5.3.15.
	script := dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": dir}}
	out, st, err = preset.Combined(t, script, `shopt -s autocd; sub; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "sub: command not found") || !strings.HasSuffix(out, dir+"\n") || st != 0 {
		t.Errorf("in a script: out %q status %d, want a command-not-found and no move", out, st)
	}
}

// A directory does not shadow a command, which is the condition that keeps
// `autocd` a fallback rather than a lookup order.
//
// Measured: with a *directory* named `echo` in the working directory and the
// option on, bash 5.3.15 still prints with the builtin.
func TestAutocdDoesNotShadowACommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "echo"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st, err := preset.Combined(t,
		dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": dir}, Interactive: true},
		`shopt -s autocd; echo hi; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "hi\n" + dir + "\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q status 0", out, st, want)
	}
}

// `shopt -s no_empty_cmd_completion` turns the completer's empty-word answer
// *off*, which is the one entry in this dialect's table whose sense is
// inverted — so the round trip is asserted against the capability rather than
// against the name.
//
// The capability starts on here: this shell answers an empty command word with
// everything that could run, so the shell is in the option's off state and
// `shopt -s` is a request to change behavior. A table that stored the option's
// own bit would have granted it by writing a `true` that meant "on".
func TestNoEmptyCmdCompletionInvertsTheCapability(t *testing.T) {
	for _, tc := range []struct {
		src           string
		wantOption    string
		wantCompletes bool
	}{
		{`shopt no_empty_cmd_completion`, "off", true},
		{`shopt -s no_empty_cmd_completion; shopt no_empty_cmd_completion`, "on", false},
		{`shopt -s no_empty_cmd_completion; shopt -u no_empty_cmd_completion; shopt no_empty_cmd_completion`, "off", true},
	} {
		var buf strings.Builder
		base := dialecttest.Base{Dir: t.TempDir(), Stdout: &buf, Stderr: &buf}
		r := preset.Runner(base)
		if _, err := r.Run(context.Background(), preset.Parse(t, tc.src)); err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		// The name is longer than the listing width, so it is written whole
		// and the tab follows it — which is what bash does with it too.
		want := "no_empty_cmd_completion\t" + tc.wantOption + "\n"
		if buf.String() != want {
			t.Errorf("%s printed %q, want %q", tc.src, buf.String(), want)
		}
		if got := r.CompletesEmptyCommandWord(); got != tc.wantCompletes {
			t.Errorf("%s: CompletesEmptyCommandWord() = %v, want %v", tc.src, got, tc.wantCompletes)
		}
	}
}

// `shopt -s cdspell` corrects a misspelled `cd`, and the test is where the
// shell ends up rather than the bit.
//
// Interactive-only in bash, measured, so the same snippet has to correct at a
// prompt and refuse in a script — which is also what keeps this out of the
// corpus, where every row is a script.
func TestCdspellCorrectsAnInteractiveCd(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	interactive := dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": t.TempDir()}, Interactive: true}
	out, st, err := preset.Combined(t, interactive, `shopt -s cdspell; cd documnets; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	want := "documents\n" + filepath.Join(dir, "documents") + "\n"
	if out != want || st != 0 {
		t.Errorf("out %q status %d, want %q status 0", out, st, want)
	}
	// Off, the same word is the ordinary refusal — which is what this shell
	// said before the option existed.
	out, st, err = preset.Combined(t, interactive, `cd documnets; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cd: documnets:") || !strings.HasSuffix(out, dir+"\n") || st != 0 {
		t.Errorf("without the option: out %q status %d, want a refusal and no move", out, st)
	}
	// And a script does not get it even with the option on.
	script := dialecttest.Base{Dir: dir, Vars: map[string]string{"PATH": t.TempDir()}}
	out, _, err = preset.Combined(t, script, `shopt -s cdspell; cd documnets; pwd`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cd: documnets:") || !strings.HasSuffix(out, dir+"\n") {
		t.Errorf("in a script: out %q, want a refusal and no move", out)
	}
}
