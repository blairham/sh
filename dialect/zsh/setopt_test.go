// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// zsh's option namespace ignores case and underscores, and a single `no`
// prefix negates: `No_Glob`, `NOGLOB` and `noglob` are one request. Measured
// 2026-09-04, zsh 5.9: each turns globbing off, and `no_no_glob` is `no such
// option` rather than glob restored — the prefix strips once.
func TestSetoptNormalizesZshSpellings(t *testing.T) {
	for _, spelling := range []string{"No_Glob", "NOGLOB", "n_o_g_l_o_b"} {
		out, st := runZsh(t, t.TempDir(), `setopt `+spelling+`; echo x*`)
		if st != 0 || !strings.Contains(out, "x*") {
			t.Errorf("setopt %s: out %q status %d, want the glob off", spelling, out, st)
		}
	}
	out, st := runZsh(t, t.TempDir(), `setopt no_no_glob; echo st=$?`)
	if !strings.Contains(out, "no such option: no_no_glob") || !strings.Contains(out, "st=1") {
		t.Errorf("out %q status %d, want a single-strip refusal at 1", out, st)
	}
}

// A name outside the namespace is refused with the operand as it was typed,
// and the other operands are still acted on — measured, in both orders.
func TestSetoptActsPastABadName(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `setopt zzqq no_glob; echo st=$?; echo x*`)
	if !strings.Contains(out, "no such option: zzqq") ||
		!strings.Contains(out, "st=1") || !strings.Contains(out, "x*") {
		t.Errorf("out %q, want the complaint, status 1, and globbing still off", out)
	}
}

// `unsetopt` is the same request inverted, from either spelling of the name.
// The failed glob then ends the run, which is zsh's own answer too.
func TestUnsetoptInverts(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt no_glob; unsetopt noglob; echo x*`)
	if st != 1 || !strings.Contains(out, "no matches found") {
		t.Errorf("out %q status %d, want globbing back on and the zsh refusal at 1", out, st)
	}
}

// Options that set -o also holds are one switch: `setopt err_exit` ends the
// script the way `set -e` does.
func TestSetoptDrivesTheSharedOptionTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt err_exit; false; echo reached`)
	if st != 1 || strings.Contains(out, "reached") {
		t.Errorf("out %q status %d, want errexit to have ended the run at 1", out, st)
	}
}

// The three axis-backed names: shwordsplit splits unquoted expansions,
// nonomatch passes a failed glob through, ksharrays bases arrays at zero.
// Each is zsh's own name for a semantics axis the vector already carries.
func TestSetoptAxisBackedOptions(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"shwordsplit", `x="a b"; setopt shwordsplit; set -- $x; echo n=$#`, "n=2"},
		{"nonomatch", `unsetopt nomatch; echo x*`, "x*"},
		{"ksharrays", `setopt ksh_arrays; a=(p q); echo ${a[0]}`, "p"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%s: out %q status %d, want %q", tc.name, out, st, tc.want)
		}
	}
}

// A subshell's setopt stays in the subshell: the vector is swapped
// copy-on-write, never mutated in place.
func TestSetoptStaysInItsSubshell(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `(setopt shwordsplit); x="a b"; set -- $x; echo n=$#`)
	if !strings.Contains(out, "n=1") {
		t.Errorf("out %q, want the parent still unsplit", out)
	}
}

// Bare `setopt` lists what differs from zsh's defaults, canonically spelled
// and ordered by the base name — measured, `noclobber` prints between
// `allexport` and `errexit`, and `nohashdirs` is the baseline's one line:
// zsh's default is to hash directories and this shell never does.
func TestBareSetoptListsTheDeviations(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt err_exit no_clobber; setopt`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if got, want := out, "noclobber\nerrexit\nnohashdirs\n"; got != want {
		t.Errorf("listing = %q, want %q", got, want)
	}
}

// An option that exists and cannot be moved answers with zsh's own wording
// for exactly that — measured on `setopt monitor` in a zsh with no terminal.
func TestSetoptRefusesWhatItCannotChange(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `setopt monitor; echo st=$?`)
	if !strings.Contains(out, "can't change option: monitor") || !strings.Contains(out, "st=1") {
		t.Errorf("out %q, want the measured refusal at 1", out)
	}
	// Asking for the state it is already in is granted, the same bargain the
	// substrate's own option table strikes for `set +o posix`.
	out, _ = runZsh(t, t.TempDir(), `unsetopt monitor; echo st=$?`)
	if !strings.Contains(out, "st=0") {
		t.Errorf("out %q, want the already-off state granted", out)
	}
}

// `setopt monitor` is granted where the shell has a terminal, which is what
// job control needs and the whole of what this option turns on.
//
// The measurement, on a pseudo-terminal against zsh 5.9.2 (2026-09-10) — the
// same three lines in an interactive shell and inside a plain `-c` string,
// which answer alike, because the axis is the terminal and not the prompt:
//
//	setopt monitor      -> 0, ${options[monitor]} is `on`, `m` in `$-`
//	unsetopt monitor    -> 0, `off`, no `m`
//	setopt monitor      -> 0, `on` again
//
// On a pipe the first of those is `can't change option: monitor` at 1 with
// the option left off, which is TestSetoptRefusesWhatItCannotChange above.
//
// This shell really is running one by then: a background command gets a
// process group of its own, the terminal is handed to whatever is in front
// and taken back afterwards, and `fg`, `bg` and `jobs` all speak about the
// table. The option was a constant wired off here until #1720, so an
// interactive shell answered `m` in `$-` and `off` in `${options[monitor]}`
// at the same moment — and a prompt theme asking for the monitor before
// starting its async worker was refused by a shell that had one.
//
// Both directions and from both starting states, because a probe that asks
// for the state the shell is already in cannot tell a switch that moves from
// a constant that was already right.
func TestSetoptMonitorNeedsATerminalAndNotAPrompt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"on, from off",
			`unsetopt monitor; setopt monitor; print "st=$? o=${options[monitor]}"`,
			"st=0 o=on\n",
		},
		{
			"off, from on",
			`setopt monitor; unsetopt monitor; print "st=$? o=${options[monitor]}"`,
			"st=0 o=off\n",
		},
		{
			"the letter follows the option",
			"unsetopt monitor; case $- in *m*) print no ;; *) print gone ;; esac\n" +
				"setopt monitor; case $- in *m*) print back ;; *) print no ;; esac",
			"gone\nback\n",
		},
		{
			"and the listing does",
			"unsetopt monitor; setopt; echo ---; setopt monitor; setopt",
			"nohashdirs\n---\nnohashdirs\nmonitor\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Terminal: true,
			}, tc.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A refused `setopt` does not end the script, where a refused `set` does.
//
// Measured on a pipe, zsh 5.9.2: `set -m; print st=$?; print DONE` writes
// `zsh:set:1: can't change option: -m` and neither `print` runs, leaving at
// 0; `setopt monitor; print st=$?; print DONE` writes
// `zsh:setopt:1: can't change option: monitor` and then `st=1` and `DONE`.
// Same option, same sentence, same status on the builtin — so what differs is
// which builtin asked, and only `set` is one of the special ones the standard
// makes fatal on error.
//
// Asserted from both sides in one test, since the fatality is a single switch
// and a change that lost it for `set` would pass the second half alone.
func TestARefusedSetoptDoesNotEndTheScript(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "setopt monitor\nprint \"st=$?\"\nprint DONE\n")
	if want := "zsh:setopt:1: can't change option: monitor\nst=1\nDONE\n"; out != want || st != 0 {
		t.Errorf("setopt: out %q status %d, want %q at 0", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), "set -m\nprint \"st=$?\"\nprint DONE\n")
	if want := "zsh:set:1: can't change option: -m\n"; out != want || st != 1 {
		t.Errorf("set: out %q status %d, want %q at 1", out, st, want)
	}
}

// The 185 names zsh 5.9.2 has are all recognized, and the sixteen a real
// `~/.zshrc` writes at startup are the reason: every one of them used to be
// `no such option`, sixteen complaints at every interactive start. None of
// them is implemented — see TestSetoptRecordsWhatItDoesNotImplement for the
// other half of that claim — and recognizing them is still the whole of the
// difference between a shell that sprays and one that starts quietly.
func TestSetoptRecognizesTheMeasuredNameSet(t *testing.T) {
	names := []string{
		"always_to_end", "auto_cd", "auto_list", "auto_menu", "complete_in_word",
		"hash_list_all", "hist_expire_dups_first", "hist_ignore_all_dups",
		"hist_ignore_space", "hist_reduce_blanks", "hist_verify", "list_ambiguous",
		"listpacked", "nocorrect", "nolisttypes", "share_history",
	}
	for _, n := range names {
		out, st := runZsh(t, t.TempDir(), `setopt `+n)
		if out != "" || st != 0 {
			t.Errorf("setopt %s: out %q status %d, want silence at 0", n, out, st)
		}
	}
	// And in one command, the way an rc file writes them.
	out, st := runZsh(t, t.TempDir(), `setopt `+strings.Join(names, " "))
	if out != "" || st != 0 {
		t.Errorf("all sixteen at once: out %q status %d, want silence at 0", out, st)
	}
}

// Recorded is not implemented, and the test says so both ways round: the
// option is remembered and reported under its canonical name, and the
// behavior the name promises does not happen.
func TestSetoptRecordsWhatItDoesNotImplement(t *testing.T) {
	// Remembered and reported. `nohashdirs` is the baseline's own line.
	out, st := runZsh(t, t.TempDir(), `setopt auto_cd; setopt`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "autocd\nnohashdirs\n"; out != want {
		t.Errorf("listing = %q, want %q", out, want)
	}
	// Turned off again, it drops out of the listing rather than lingering.
	out, _ = runZsh(t, t.TempDir(), `setopt auto_cd; unsetopt auto_cd; setopt`)
	if want := "nohashdirs\n"; out != want {
		t.Errorf("listing after unsetopt = %q, want %q", out, want)
	}
	// Not implemented: with autocd recorded and on, naming a directory still
	// does not change directory. The complaint is put aside because what it
	// says depends on how the name resolves; where the shell ends up does
	// not, and that is the claim.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, st = runZsh(t, dir, `setopt auto_cd; sub 2>/dev/null; pwd`)
	if want := dir + "\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q — autocd is recorded, not implemented", out, st, want)
	}
}

// The twelve sh and ksh spellings are second names for options the table
// already holds, never entries of their own: they share the canonical state
// exactly, and a listing always prints the canonical name. Measured, one
// flip at a time, against zsh 5.9.2.
func TestSetoptCompatSpellingsShareTheCanonicalState(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`setopt dotglob; setopt`, "globdots\nnohashdirs\n"},
		{`setopt trackall; setopt`, "hashcmds\nnohashdirs\n"},
		{`setopt hashall; setopt`, "hashcmds\nnohashdirs\n"},
		{`setopt nolog; setopt`, "nohashdirs\nhistnofunctions\n"},
		// The canonical name and the compat one are one switch, so setting
		// through one and clearing through the other leaves nothing behind.
		{`setopt dotglob; unsetopt globdots; setopt`, "nohashdirs\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
	// `braceexpand` and `physical` are fixed here rather than recorded —
	// this shell always expands braces and never resolves a symbolic link on
	// the way to a directory — and their canonical names are fixed with
	// them, which is the sharpest evidence that an alias is the same entry:
	// both spellings refuse identically, each echoing the one that was
	// typed. Real zsh grants both, and docs/spec/semantics.md records that.
	for _, tc := range []struct{ src, want string }{
		{`setopt no_braceexpand`, "zsh:setopt:1: can't change option: no_braceexpand\n"},
		{`setopt ignorebraces`, "zsh:setopt:1: can't change option: ignorebraces\n"},
		{`setopt physical`, "zsh:setopt:1: can't change option: physical\n"},
		{`setopt chaselinks`, "zsh:setopt:1: can't change option: chaselinks\n"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != 1 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q at 1", tc.src, out, st, tc.want)
		}
	}
}

// Five of the 185 refuse to move in a `-c` shell and they are the five about
// being interactive — measured by asking a real zsh for every one of the 197
// spellings in both directions, which refused these and granted the rest.
// Both of the compat spellings for them refuse identically.
func TestSetoptRefusesTheInteractiveOnlyOptions(t *testing.T) {
	for _, name := range []string{
		"interactive", "monitor", "shinstdin", "singlecommand", "zle",
		"onecmd", "stdin",
	} {
		out, st := runZsh(t, t.TempDir(), `setopt `+name)
		want := "zsh:setopt:1: can't change option: " + name + "\n"
		if out != want || st != 1 {
			t.Errorf("setopt %s: out %q status %d, want %q at 1", name, out, st, want)
		}
		// Off is where they already are, so asking for that is granted.
		out, st = runZsh(t, t.TempDir(), `unsetopt `+name)
		if out != "" || st != 0 {
			t.Errorf("unsetopt %s: out %q status %d, want silence at 0", name, out, st)
		}
	}
}

// Three of the names new to the table are implemented rather than recorded,
// because the pattern matcher already carries the behavior each one asks
// for. Measured against zsh 5.9.2: `caseglob` governs pathname expansion
// alone — `casematch` is about `=~` and is recorded, not wired to this.
func TestSetoptImplementsTheGlobOptions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "B.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		// nullglob deletes a word that matched nothing, and it wins over
		// nomatch: no complaint, status 0.
		{`setopt nullglob; echo "[" zz* "]"`, "[ ]\n"},
		{`setopt globdots; echo *`, ".hidden B.txt a.txt\n"},
		{`echo *`, "B.txt a.txt\n"},
		{`unsetopt caseglob; echo b*`, "B.txt\n"},
		// caseglob is the glob's alone: a case statement still compares
		// letters as written.
		{`unsetopt caseglob; case AB in ab) echo yes;; *) echo no;; esac`, "no\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if st != 0 || out != tc.want {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
}

// `setopt nounset` was refused where `set -o nounset` was granted, though
// both name the one switch the substrate really applies. Measured on the
// real shell: it is granted there, in both directions.
func TestSetoptNounsetIsTheSameSwitchAsSetU(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt nounset; echo "${zz}"`)
	if want := "zsh:1: zz: parameter not set\n"; out != want || st != 1 {
		t.Errorf("out %q status %d, want %q at 1", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `unsetopt unset; echo "${zz}"`)
	if want := "zsh:1: zz: parameter not set\n"; out != want || st != 1 {
		t.Errorf("unsetopt unset: out %q status %d, want %q at 1", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `setopt unset; echo "[${zz}]"`)
	if want := "[]\n"; out != want || st != 0 {
		t.Errorf("setopt unset: out %q status %d, want %q at 0", out, st, want)
	}
}

// A bare `unsetopt` is the complement of a bare `setopt`: every option in the
// spelling that is off by default, printed when it is off. 184 of the 185 in
// a shell that has changed nothing, because `hashdirs` is the one that
// deviates — the same one line `setopt` prints. Measured: real zsh prints 184
// there too.
func TestBareUnsetoptListsWhatIsOff(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `unsetopt`)
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 184 {
		t.Errorf("listing has %d lines, want 184", len(lines))
	}
	if lines[0] != "noaliases" || lines[len(lines)-1] != "zle" {
		t.Errorf("listing runs %q … %q, want noaliases … zle", lines[0], lines[len(lines)-1])
	}
	for _, l := range lines {
		if l == "nohashdirs" {
			t.Errorf("nohashdirs is on, so it belongs to setopt's listing and not to this one")
		}
	}
	// Setting one takes it out of this listing and puts it in the other's.
	out, _ = runZsh(t, t.TempDir(), `setopt auto_cd; unsetopt`)
	for _, l := range strings.Split(out, "\n") {
		if l == "autocd" {
			t.Errorf("autocd is on and still listed by unsetopt")
		}
	}
}

// A recorded option is runner state like any other, so a subshell's stays in
// the subshell — the store is a parameter the clone deep-copies rather than a
// package variable it would share.
func TestRecordedOptionStaysInItsSubshell(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `(setopt auto_cd); setopt`)
	if want := "nohashdirs\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q — the parent kept the subshell's change", out, st, want)
	}
}

// An emulation resets every changeable option to its defaults, and the
// recorded ones are changeable options: `emulate zsh` clears them, and
// `emulate sh -c` puts back what was set before it.
func TestEmulateResetsAndRestoresRecordedOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt auto_cd; emulate zsh; setopt`)
	if want := "nohashdirs\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), `setopt auto_cd; emulate sh -c ':'; setopt`)
	if want := "autocd\nnohashdirs\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}

// `[[ -o name ]]` reads the same namespace `setopt` writes, so widening the
// table widened the condition with it: a name that is only recorded still
// answers, and the `no` prefix still inverts. Measured against zsh 5.9.2,
// which gives the identical four answers.
func TestRecordedOptionAnswersTheCondition(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`[[ -o auto_cd ]]; echo "a=$?"; setopt auto_cd; [[ -o auto_cd ]]; echo "b=$?"; `+
			`[[ -o no_auto_cd ]]; echo "c=$?"; [[ -o dotglob ]]; echo "d=$?"`)
	if want := "a=1\nb=0\nc=1\nd=1\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}

// TestAliasesIsAnOptionAndNotAConstant — `no_aliases` is what a prompt theme
// sets to protect its own code from a user's aliases, and it was a name that
// could be read and not written: `setopt aliases` was granted because the
// state already matched, and `setopt no_aliases` was `can't change option`.
//
// The state is the *option*, which is not the same question as whether this
// shell expands aliases at all. The route answers that one — measured, zsh
// under `-c` does not expand and the same two lines in a file do — while
// `[[ -o aliases ]]` reads on under `-c` all the same.
func TestAliasesIsAnOptionAndNotAConstant(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`[[ -o aliases ]]; echo on=$?`, "on=0\n"},
		{`setopt no_aliases; [[ -o aliases ]]; echo off=$?`, "off=1\n"},
		{`unsetopt aliases; [[ -o aliases ]]; echo off=$?`, "off=1\n"},
		{`unsetopt aliases; setopt aliases; [[ -o aliases ]]; echo back=$?`, "back=0\n"},
		// Through `builtin`, which is how the theme writes it.
		{`builtin setopt no_aliases; [[ -o aliases ]]; echo off=$?`, "off=1\n"},
	} {
		if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
	// The listing follows, and it is the half a state nothing reads would
	// still pass: `setopt` prints the spellings that are on where their
	// default is off, so `noaliases` appears there only once it has been
	// set, and `unsetopt` prints it until then. Asserted as whole lines
	// rather than by searching the text, because `noaliases` is a substring
	// of nothing here but would be of a name added later.
	for _, tc := range []struct {
		src, line string
		want      bool
	}{
		{`setopt`, "noaliases", false},
		{`setopt no_aliases; setopt`, "noaliases", true},
		{`unsetopt`, "noaliases", true},
		{`setopt no_aliases; unsetopt`, "noaliases", false},
	} {
		out, st := runZsh(t, dir, tc.src)
		if st != 0 {
			t.Errorf("%s: status %d", tc.src, st)
		}
		if got := hasLine(out, tc.line); got != tc.want {
			t.Errorf("%s: %q listed = %v, want %v (out %q)", tc.src, tc.line, got, tc.want, out)
		}
	}
}

// hasLine reports whether s has line as a whole line of its own.
func hasLine(s, line string) bool {
	for _, l := range strings.Split(s, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// TestNoAliasesReachesTheExpansionSwitch — the option is not only recorded:
// turning it off stops the parser being offered the table, and turning it
// back on restores what the *route* said rather than forcing expansion on.
//
// Asserted through the runner rather than through a program, because the
// helper above parses its whole source before running any of it — so a
// `setopt` in one snippet can never be seen by the parse of the next line in
// the same snippet. The corpus case runs it from a file, where it can.
func TestNoAliasesReachesTheExpansionSwitch(t *testing.T) {
	for _, base := range []bool{true, false} {
		r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
		r.SetAliasExpansionBase(base)
		if got := r.AliasExpansion(); got != base {
			t.Fatalf("base %v: the switch started at %v", base, got)
		}
		setopt, ok := r.Builtin("setopt")
		if !ok {
			t.Fatal("this dialect has no setopt")
		}
		if code := setopt(r, t.Context(), []string{"no_aliases"}); code != 0 {
			t.Errorf("base %v: setopt no_aliases reported %d", base, code)
		}
		if r.AliasExpansion() {
			t.Errorf("base %v: the option is off and the switch is still on", base)
		}
		if code := setopt(r, t.Context(), []string{"aliases"}); code != 0 {
			t.Errorf("base %v: setopt aliases reported %d", base, code)
		}
		// Back to the route's answer, not to `true`: a shell the route says
		// does not expand must not start expanding because a script turned
		// an option back on.
		if got := r.AliasExpansion(); got != base {
			t.Errorf("base %v: the switch came back at %v, want the route's answer", base, got)
		}
	}
}
