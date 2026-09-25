// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A bare `emulate` names the current mode, and the mode is what the last
// emulation set — measured, `zsh` before anything and the word after.
func TestBareEmulateNamesTheMode(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `emulate; emulate sh; emulate; emulate ksh; emulate`)
	if st != 0 || out != "zsh\nsh\nksh\n" {
		t.Errorf("out %q status %d, want the three modes in turn", out, st)
	}
}

// `emulate sh` moves the three measured axes: unquoted expansions split, a
// failed glob passes through, and arrays base at zero; `emulate zsh` puts all
// three back.
func TestEmulateShMovesTheMeasuredAxes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`x="a b"; set -- $x; echo n=$#; emulate sh; set -- $x; echo n=$#; echo x*; `+
			`a=(p q); echo ${a[1]}; emulate zsh; set -- $x; echo n=$#`)
	if st != 0 || out != "n=1\nn=2\nx*\nq\nn=1\n" {
		t.Errorf("out %q status %d, want split, passthrough, zero base, and back", out, st)
	}
}

// The fourth axis, and the one that says where the answer to a POSIX-mode
// question lives: under `emulate sh` a failed redirection on a special
// builtin ends the script, and under `emulate zsh` it is a complaint the
// script runs past. Measured on the real shell, which stops for `emulate sh`
// and for `emulate ksh` alike and carries on for `emulate zsh`.
func TestEmulateShEndsTheScriptOnAFailedRedirection(t *testing.T) {
	for _, mode := range []string{"sh", "ksh"} {
		out, st := runZsh(t, t.TempDir(), `emulate `+mode+`; exec 3>/nope/x; echo after`)
		if strings.Contains(out, "after") {
			t.Errorf("emulate %s: out %q, want the script stopped at the redirection", mode, out)
		}
		if st != 1 {
			t.Errorf("emulate %s: status %d, want this shell's fatal status", mode, st)
		}
	}
	out, st := runZsh(t, t.TempDir(), `emulate zsh; exec 3>/nope/x; echo after`)
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want zsh's own answer, which carries on", out, st)
	}
	// And without any emulate at all, which is the same answer by default.
	out, st = runZsh(t, t.TempDir(), `exec 3>/nope/x; echo after`)
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want the preset's own answer", out, st)
	}
}

// A plain emulation resets options to its defaults — measured, no -R needed:
// `setopt err_exit; emulate zsh` turns errexit back off.
func TestEmulateResetsOptions(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt err_exit; emulate zsh; false; echo reached`)
	if st != 0 || !strings.Contains(out, "reached") {
		t.Errorf("out %q status %d, want errexit gone", out, st)
	}
}

// TestEmulateLeavesTheOptionsItDoesNotOwn — and it resets *some* options
// rather than all of them, which TestEmulateResetsOptions cannot show on its
// own.
//
// Measured on zsh 5.9.2, one name at a time: a bare emulation puts 81 of the
// 185 names back to a default and leaves the other 104 where the script left
// them. Every case below pairs a name from the 104 with one from the 81, and
// the pairing is the point — a shell that had stopped emulating altogether
// would pass the first half of each, and the pre-#2515 shell passed the
// second.
func TestEmulateLeavesTheOptionsItDoesNotOwn(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		// The one a session reads before it records a line, which is why
		// #2515 was filed against a live option rather than a hypothesis.
		{
			`setopt histignorespace; setopt extendedglob; emulate sh; ` +
				`[[ -o histignorespace ]]; echo "kept=$?"; [[ -o extendedglob ]]; echo "gone=$?"`,
			"kept=0\ngone=1\n",
		},
		// The one the divergence was first noticed on.
		{
			`setopt nopromptsp; setopt nullglob; emulate sh; ` +
				`[[ -o promptsp ]]; echo "sp=$?"; [[ -o nullglob ]]; echo "gone=$?"`,
			"sp=1\ngone=1\n",
		},
		// Nothing about which keys the person at the keyboard presses is
		// portability.
		{
			`setopt vi; setopt warncreateglobal; emulate ksh; ` +
				`[[ -o vi ]]; echo "vi=$?"; [[ -o warncreateglobal ]]; echo "gone=$?"`,
			"vi=0\ngone=1\n",
		},
		// `emulate zsh` partitions the table the same way, which is what says
		// the set belongs to the option rather than to the emulation.
		{
			`setopt correct; setopt kshglob; emulate zsh; ` +
				`[[ -o correct ]]; echo "kept=$?"; [[ -o kshglob ]]; echo "gone=$?"`,
			"kept=0\ngone=1\n",
		},
	} {
		if out, st := runZsh(t, dir, tc.src); out != tc.want || st != 0 {
			t.Errorf("%s: out %q status %d, want %q", tc.src, out, st, tc.want)
		}
	}
}

// TestEmulateDashRResetsWhatABareEmulationLeaves — the letter is what the
// wider set is for.
//
// It was read here as adding nothing, on the strength of a bare emulation
// already resetting the whole table. With the bare form narrowed to the 81,
// `-R` is the form that reaches the rest: every name but the nine describing
// how the shell was started.
func TestEmulateDashRResetsWhatABareEmulationLeaves(t *testing.T) {
	dir := t.TempDir()
	// The same option as the first row above and the opposite answer, which
	// is what says the two forms differ rather than that the option moved.
	out, st := runZsh(t, dir, `setopt histignorespace; emulate -R sh; [[ -o histignorespace ]]; echo "gone=$?"`)
	if out != "gone=1\n" || st != 0 {
		t.Errorf("out %q status %d, want the option back at its default", out, st)
	}
	out, _ = runZsh(t, dir, `setopt histignorespace; emulate sh; [[ -o histignorespace ]]; echo "kept=$?"`)
	if out != "kept=0\n" {
		t.Errorf("out %q, want a bare emulation to leave the same option alone", out)
	}
	// And the nine stand through the strict form too — `login` here, which a
	// script may move in both directions and no emulation puts back.
	out, _ = runZsh(t, dir, `setopt login; emulate -R zsh; [[ -o login ]]; echo "still=$?"`)
	if out != "still=0\n" {
		t.Errorf("out %q, want the shell's own state left standing", out)
	}
}

// TestEmulateDashLResetsTheSameSetAsABareOne — `-L` scopes the emulation to
// the call and does not widen or narrow what it resets.
//
// Worth its own case because the letter looks like it might: it is the form a
// prompt theme opens every function with, so a wrong set here is wrong on
// every function call rather than once in an rc file.
func TestEmulateDashLResetsTheSameSetAsABareOne(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { setopt histignorespace extendedglob; emulate -L sh; `+
			`[[ -o histignorespace ]]; echo "kept=$?"; [[ -o extendedglob ]]; echo "gone=$?"; }; f`)
	if out != "kept=0\ngone=1\n" || st != 0 {
		t.Errorf("out %q status %d, want the same 81 back and the same 104 untouched", out, st)
	}
}

// A word naming no emulation is passed over in silence, the mode unchanged —
// measured on `fish` and on `SH`, whose case does not match.
func TestEmulateIgnoresAnUnknownMode(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `emulate fish; echo st=$?; emulate SH; emulate`)
	if st != 0 || out != "st=0\nzsh\n" {
		t.Errorf("out %q status %d, want silence and the mode still zsh", out, st)
	}
}

// `-c` runs the string under the emulation and restores everything after,
// options included — measured, a `no_glob` set before it comes back.
func TestEmulateDashCRestores(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`setopt no_glob; emulate sh -c 'x="a b"; set -- $x; echo n=$#'; echo x*; `+
			`x="c d"; set -- $x; echo n=$#; emulate`)
	if st != 0 || out != "n=2\nx*\nn=1\nzsh\n" {
		t.Errorf("out %q status %d, want the emulation inside and everything back after", out, st)
	}
}

// The refusals, each measured: a bad option letter, a `-c` whose string never
// arrives, a second operand, flags with no mode, and a name the option table
// does not have.
//
// `-L` is not among them and has not been for a while — the letter is taken
// and honored. The comment here said it was refused for want of a seam to
// restore on return; localoptions.go is that seam now, and the letter is one
// `setopt` on top of the emulation. See localoptions_test.go for its rows.
func TestEmulateRefusals(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		status    int
	}{
		{`emulate -x sh`, "bad option: -x", 1},
		{`emulate sh -c`, "string expected after -c", 1},
		{`emulate zsh sh`, "unknown argument sh", 1},
		{`emulate -R`, "not enough arguments", 1},
		{`emulate zsh -o nosuchopt`, "no such option: nosuchopt", 1},
		{`emulate zsh -o`, "string expected after -o", 1},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if st != tc.status || !strings.Contains(out, tc.want) {
			t.Errorf("%s: out %q status %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
		}
	}
}

// An emulation in a subshell stays there: the semantics swap is
// copy-on-write and the mode lives in the variable table a clone copies.
func TestEmulateStaysInItsSubshell(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `(emulate sh); emulate; x="a b"; set -- $x; echo n=$#`)
	if st != 0 || out != "zsh\nn=1\n" {
		t.Errorf("out %q status %d, want the parent untouched", out, st)
	}
}

// TestEmulateTakesAnOptionByName — `{+|-}o name` names an option for the
// emulation to apply after its own defaults are in place. It is the form a
// real prompt theme opens every one of its functions with, and the name is
// `setopt`'s: underscores and all, and refused by `setopt`'s sentence when it
// is not a name at all.
func TestEmulateTakesAnOptionByName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
		st   int
	}{
		{`emulate zsh -o extendedglob; [[ -o extendedglob ]]; echo on=$?`, "on=0\n", 0},
		// The same option written with an underscore, which is the spelling
		// the theme uses.
		{`emulate zsh -o extended_glob; [[ -o extendedglob ]]; echo on=$?`, "on=0\n", 0},
		// `+o` is the same request the other way.
		{`setopt extendedglob; emulate zsh +o extendedglob; [[ -o extendedglob ]]; echo off=$?`, "off=1\n", 0},
		// Applied *after* the emulation's own reset, which is the ordering
		// that matters: a plain emulation puts every option back to its
		// default, so an option placed first would be undone.
		{`emulate zsh -o err_exit; [[ -o err_exit ]]; echo on=$?`, "on=0\n", 0},
	} {
		if out, st := runZsh(t, dir, tc.src); out != tc.want || st != tc.st {
			t.Errorf("%s: out %q status %d, want %q status %d", tc.src, out, st, tc.want, tc.st)
		}
	}
}

// TestEmulateRefusesAnOptionItCannotName — the two ways `-o` goes wrong, each
// asserted on the whole rendered line, because what is being pinned is the
// sentence and not only the status.
func TestEmulateRefusesAnOptionItCannotName(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir, `emulate zsh -o nosuchopt; echo st=$?`)
	if !strings.Contains(out, "emulate:1: no such option: nosuchopt\n") || !strings.Contains(out, "st=1\n") {
		t.Errorf("out %q status %d, want the option named and 1", out, st)
	}
	out, _ = runZsh(t, dir, `emulate zsh -o; echo st=$?`)
	if !strings.Contains(out, "emulate:1: string expected after -o\n") || !strings.Contains(out, "st=1\n") {
		t.Errorf("out %q, want the missing argument named", out)
	}
	// An option with no emulation to apply it to. Measured: real zsh answers
	// `bad option: -o` here rather than complaining about the count.
	out, _ = runZsh(t, dir, `emulate -o extendedglob; echo st=$?`)
	if !strings.Contains(out, "emulate:1: bad option: -o\n") || !strings.Contains(out, "st=1\n") {
		t.Errorf("out %q, want the letter refused where nothing is being emulated", out)
	}
}

// TestEmulateDashLLastsAsLongAsTheFunction — `-L` is LOCAL_OPTIONS: the
// emulation and every option moved after it are put back when the call
// unwinds.
//
// Written as a function on purpose. Outside one the letter changes nothing —
// measured, `emulate -L zsh -o extendedglob` at the top level leaves the
// option on afterwards — so a test at the top level would pass with the
// letter ignored, which is what it did before.
func TestEmulateDashLLastsAsLongAsTheFunction(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir,
		`f() { emulate -L zsh -o extendedglob; [[ -o extendedglob ]]; echo in=$?; }; f; `+
			`[[ -o extendedglob ]]; echo out=$?`)
	if out != "in=0\nout=1\n" || st != 0 {
		t.Errorf("out %q status %d, want the option on inside and off after", out, st)
	}
	// The *call* is what is restored rather than the emulation's own
	// changes: an option set later in the same function goes back too, which
	// is the whole of what the option is for.
	out, _ = runZsh(t, dir,
		`f() { emulate -L zsh; setopt err_exit; [[ -o err_exit ]]; echo in=$?; }; f; `+
			`[[ -o err_exit ]]; echo out=$?`)
	if out != "in=0\nout=1\n" {
		t.Errorf("out %q, want a later setopt restored as well", out)
	}
	// And an option the *caller* had set before the call, which is what says
	// the snapshot is taken before the emulation rather than after it: a
	// plain emulation resets every option, so a state saved afterwards would
	// restore the defaults instead of what the caller had.
	out, _ = runZsh(t, dir, `setopt err_exit; f() { emulate -L zsh; }; f; [[ -o err_exit ]]; echo after=$?`)
	if out != "after=0\n" {
		t.Errorf("out %q, want the caller's own option back on", out)
	}
	// And the mode itself.
	out, _ = runZsh(t, dir, `f() { emulate -L sh; emulate; }; f; emulate`)
	if out != "sh\nzsh\n" {
		t.Errorf("out %q, want the mode local to the call", out)
	}
	// An anonymous function is a function, which is the shape the theme
	// actually uses.
	out, _ = runZsh(t, dir,
		`(){ emulate -L zsh -o extendedglob; [[ -o extendedglob ]]; echo in=$?; }; `+
			`[[ -o extendedglob ]]; echo out=$?`)
	if out != "in=0\nout=1\n" {
		t.Errorf("out %q, want an anonymous function to be a scope too", out)
	}
	// Outside a function the letter changes nothing, and that is measured
	// rather than a convenience: nothing to restore to is not a failure.
	out, st = runZsh(t, dir, `emulate -L zsh -o extendedglob; [[ -o extendedglob ]]; echo top=$?`)
	if out != "top=0\n" || st != 0 {
		t.Errorf("out %q status %d, want the option left on at the top level", out, st)
	}
}

// The fifth axis, and the only one an emulation carries that a script can
// also ask for by its own name: `posixbuiltins` decides whether `command`
// reaches a builtin, and `emulate sh` and `emulate ksh` turn it on where
// `emulate csh` and `emulate zsh` leave it off.
//
// `set -f` is the probe because nothing on any PATH is called `set`, so
// reading `$-` afterwards tells the two answers apart without depending on
// what the machine happens to have installed.
func TestEmulateTurnsPosixBuiltinsOnForTheShFamily(t *testing.T) {
	const probe = `command set -f; echo "st=$?"; case $- in (*f*) echo SET;; (*) echo UNSET;; esac`
	for _, mode := range []string{"sh", "ksh"} {
		out, st := runZsh(t, t.TempDir(), `emulate `+mode+`; `+probe)
		if st != 0 || out != "st=0\nSET\n" {
			t.Errorf("emulate %s: out %q status %d, want the builtin reached", mode, out, st)
		}
	}
	for _, mode := range []string{"csh", "zsh"} {
		out, _ := runZsh(t, t.TempDir(), `emulate `+mode+`; `+probe)
		if !strings.Contains(out, "st=127") || !strings.Contains(out, "UNSET") {
			t.Errorf("emulate %s: out %q, want the word to have asked for an external", mode, out)
		}
	}
	// And it is the option that carries it, not the mode: turning the option
	// off again after the emulation puts zsh's own answer back.
	out, _ := runZsh(t, t.TempDir(), `emulate sh; unsetopt posixbuiltins; `+probe)
	if !strings.Contains(out, "st=127") || !strings.Contains(out, "UNSET") {
		t.Errorf("out %q, want the option to have decided it", out)
	}
}

// An emulation puts each name it resets at **that emulation's** default
// rather than at zsh's, which is #2549: it picked the right set of names in
// #2515 and then put 49 of the 81 a bare `emulate sh` resets on the wrong
// value.
//
// The three controls #2515 established, run through the shell rather than
// read off the table: a name whose `sh` default differs from zsh's, one where
// `ksh` and `sh` part company, and one all four agree about. Measured on
// zsh 5.9.2, 2026-09-13, by reading `${options[name]}` after
// `emulate -R <mode>` in a fresh shell.
func TestAnEmulationLeavesTheEmulationsOwnDefaults(t *testing.T) {
	// `[[ -o … ]]` answers 0 for on, so the strings below read on/off.
	probe := func(names ...string) string {
		src := ""
		for _, n := range names {
			src += `[[ -o ` + n + ` ]] && echo "` + n + ` on" || echo "` + n + ` off"; `
		}
		return src
	}
	names := []string{"multios", "posixbuiltins", "kshglob", "cshnullcmd", "extendedglob"}
	for _, tc := range []struct{ mode, want string }{
		{"zsh", "multios on\nposixbuiltins off\nkshglob off\ncshnullcmd off\nextendedglob off\n"},
		{"sh", "multios off\nposixbuiltins on\nkshglob off\ncshnullcmd off\nextendedglob off\n"},
		{"ksh", "multios off\nposixbuiltins on\nkshglob on\ncshnullcmd off\nextendedglob off\n"},
		{"csh", "multios off\nposixbuiltins off\nkshglob off\ncshnullcmd on\nextendedglob off\n"},
	} {
		out, st := runZsh(t, t.TempDir(), `emulate `+tc.mode+`; `+probe(names...))
		if st != 0 || out != tc.want {
			t.Errorf("emulate %s: out %q status %d, want %q", tc.mode, out, st, tc.want)
		}
	}
	// And from the other side: a name the script moved *away* from the
	// emulation's default comes back to the emulation's, not to zsh's. Two
	// directions, because a reset that only ever turns things off would pass
	// the first half of this on its own.
	out, st := runZsh(t, t.TempDir(),
		`setopt multios; unsetopt posixbuiltins; emulate sh; `+probe("multios", "posixbuiltins"))
	if want := "multios off\nposixbuiltins on\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
	// `emulate csh` used to skip the axis swap outright, so the four names
	// that rode it kept whatever the script had left. Measured: csh's value
	// for them is zsh's, so the skip stood for nothing and its cost was that
	// csh alone did not reset them.
	out, st = runZsh(t, t.TempDir(),
		`setopt shwordsplit ksharrays posixbuiltins; unsetopt nomatch; emulate csh; `+
			probe("shwordsplit", "ksharrays", "posixbuiltins", "nomatch"))
	if want := "shwordsplit off\nksharrays off\nposixbuiltins off\nnomatch on\n"; st != 0 || out != want {
		t.Errorf("emulate csh: out %q status %d, want %q", out, st, want)
	}
}

// And a name whose base state the *invocation* decided comes back with the
// rest, which is #4506 — here through the **builtin** rather than through
// applyEmulation, because the letters are where an emulation is composed and
// a reset that is right on its own can still be wrong once something scopes
// it. `-L` scopes it to the function, `-c` restores after the string, and
// each of those saves and puts back the same store the reset writes to.
//
// `hashdirs` is off in a script and on in real zsh's default, so a script
// shell that emulates strictly is where the two readings part: real zsh turns
// it on and this shell used to leave it off. Measured on zsh 5.9.2 at
// `/opt/homebrew/bin/zsh`, 2026-09-25, every row below run as `zsh -f` over a
// script — with `rcs` reading identically to `hashdirs` in all of them, which
// this harness cannot ask for because it does not suppress startup files.
func TestAnEmulationResetsAnInvocationDecidedNameEndToEnd(t *testing.T) {
	const read = `[[ -o hashdirs ]] && print -r on || print -r off`
	for _, tc := range []struct{ name, src, want string }{
		// The reset, in every mode.
		{"-R zsh", `emulate -R zsh; ` + read, "on\n"},
		{"-R sh", `emulate -R sh; ` + read, "on\n"},
		{"-R ksh", `emulate -R ksh; ` + read, "on\n"},
		{"-R csh", `emulate -R csh; ` + read, "on\n"},
		// The strict form is what reaches it: the name is in the 95.
		{"a bare emulation leaves it", `emulate zsh; ` + read, "off\n"},
		{"and a bare sh leaves it", `emulate sh; ` + read, "off\n"},
		// The route control: the same answer whether the shell's own kind
		// put the name there or a `setopt` did.
		{"a setopt before it makes no difference", `setopt hashdirs; emulate -R zsh; ` + read, "on\n"},
		{"nor an unsetopt", `unsetopt hashdirs; emulate -R zsh; ` + read, "on\n"},
		// `-L` scopes the reset to the function and does not narrow it.
		{
			"-LR resets inside and restores at the return",
			`f() { emulate -LR zsh; ` + read + ` }; f; ` + read,
			"on\noff\n",
		},
		{
			"-L alone leaves it, being a bare emulation",
			`f() { emulate -L zsh; ` + read + ` }; f; ` + read,
			"off\noff\n",
		},
		{
			"-R without -L resets and stays reset",
			`f() { emulate -R zsh; ` + read + ` }; f; ` + read,
			"on\non\n",
		},
		// `-c` runs the string under the emulation and puts everything back.
		{
			"-c resets for the string and restores after it",
			`emulate -R sh -c '` + read + `'; ` + read,
			"on\noff\n",
		},
		// And the listing, which is where it shows without anybody asking:
		// a bare `setopt` prints deviations, so a name put back at its
		// default leaves the listing rather than joining it. Real zsh
		// prints `nohashdirs` before the emulation and nothing after.
		{"the listing loses the row", `setopt; print -r -- ---; emulate -R zsh; setopt`, "nohashdirs\n---\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if st != 0 || out != tc.want {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
