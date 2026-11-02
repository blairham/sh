// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package cross_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A `( … )` or `$( … )` that sends the shell a signal the shell has trapped,
// read through each preset: interp.Semantics.SubshellEndsWhenItSendsTheShellATrappedSignal
// (#7168).
//
// ksh93 runs both in the shell's own process, so the signal lands on the
// subshell: it ends there at 256 + N, every unforked subshell around it with
// it, and the handler runs in the shell afterwards. bash, zsh and dash run
// the subshell in a child and it runs on. Measured 2026-11-01 under `-c`, five
// runs each, ksh93u+ 2012-08-01, bash 5.3.20, zsh 5.9.2 and dash, `trap 'echo
// T' USR1` in front of the rows that do not set their own traps:
//
//	(kill -USR2 $$; echo in1; kill -HUP $$; …)  ksh93 USR2, after 287; bash in1 … HUP USR1 USR2, after 0
//	(kill -USR1 $$; echo in)                    ksh93 T, after 286; bash, zsh, dash in, T, after 0
//	{ kill -USR1 $$; echo in; }                 T, in, after 0 in all four
//	x=$(kill -USR1 $$; echo in)                 ksh93 T, after 286, x empty
//	( (kill -USR1 $$; echo in1); echo in2 )     ksh93 T, after 286
//	( x=$(kill -USR1 $$; echo in); echo in2 )   ksh93 T, after 286
//
// The rows after them are recorded and not modeled — docs/spec/semantics.md,
// "And a signal the shell has trapped" — and what is pinned is this shell's
// own answer:
//
//	(exit 5); (kill …), handler `echo T $?`      ksh93 T 5, after 286
//	(trap 'echo S' USR1; kill -USR1 $$; …)       ksh93 Trace/BPT trap, after 261
//	(kill -USR1 $$; echo in) | /bin/cat          ksh93 T, in, after 0
func TestASubshellEndsOnATrappedSignalToTheShellInOneDialect(t *testing.T) {
	const trap = `trap 'echo T' USR1; `
	killed := strconv.Itoa(256 + int(syscall.SIGUSR1))
	issue := `for s in HUP USR1 USR2; do trap "echo $s" $s; done; ` +
		`(kill -USR2 $$; echo in1; kill -HUP $$; echo in2; kill -USR1 $$; echo in3); echo "after $?"`
	simple := trap + `(kill -USR1 $$; echo in); echo "after $?"`
	for _, c := range []struct{ dialect, src, want string }{
		{"ksh", issue, "USR2 after " + strconv.Itoa(256+int(syscall.SIGUSR2))},
		{"bash", issue, "in1 in2 in3 HUP USR1 USR2 after 0"},
		{"ksh", simple, "T after " + killed},
		{"bash", simple, "in T after 0"},
		{"zsh", simple, "in T after 0"},
		{"dash", simple, "in T after 0"},
		// The pid held fixed, the subshell taken away.
		{"ksh", trap + `{ kill -USR1 $$; echo in; }; echo "after $?"`, "T in after 0"},
		{"ksh", trap + `x=$(kill -USR1 $$; echo in); echo "after $? x=$x"`, "T after " + killed + " x="},
		// Every unforked subshell around it ends too.
		{"ksh", trap + `( (kill -USR1 $$; echo in1); echo in2 ); echo "after $?"`, "T after " + killed},
		{"ksh", trap + `( x=$(kill -USR1 $$; echo in); echo in2 ); echo "after $?"`, "T after " + killed},
		// Recorded, not modeled: this shell's answers.
		{
			"ksh", `trap 'echo T $?' USR1; (exit 5); (kill -USR1 $$; echo in); echo "after $?"`,
			"T " + killed + " after " + killed,
		},
		{"ksh", trap + `(trap 'echo S' USR1; kill -USR1 $$; echo in); echo "after $?"`, "T after " + killed},
		{"ksh", trap + `(kill -USR1 $$; echo in) | /bin/cat; echo "after $?"`, "in T after 0"},
	} {
		out, st, err := presets[c.dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatalf("%s: %s: %v", c.dialect, c.src, err)
		}
		if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
			t.Errorf("%s: %s\n got %q at %d\nwant %q at 0", c.dialect, c.src, got, st, c.want)
		}
	}
}

// And the subshell's own EXIT trap, which a subshell ended where its signal
// landed does not run, nor any unforked subshell around it (#7153): the
// process that would run one is the shell. Measured 2026-11-01 under `-c`,
// each shell in a process group of its own, ksh93u+ 2012-08-01, bash 5.3.20,
// zsh 5.9.2 and dash, `trap 'echo bye' EXIT` in front of each row:
//
//	(trap 'echo sub' EXIT; kill -TERM $$; echo s2)  ksh93 bye; bash s2, sub, bye; zsh, dash s2, sub
//	x=$(trap 'echo sub >&2' EXIT; kill -TERM $$; …) ksh93 bye
//	: | (trap 'echo sub' EXIT; kill -TERM $$; …)    ksh93 bye
//	( (trap … EXIT; kill -TERM $$); echo s2 )       ksh93 bye
//	(trap 'echo sub' EXIT; kill -SEGV $$; echo s2)  ksh93 nothing
//	the first row with USR1 trapped                 ksh93 T, after 286, bye
//
// each killed by the signal but the last. Here ksh wrote `sub` before `bye`.
func TestASubshellEndedWhereItsSignalLandedRunsNoExitTrap(t *testing.T) {
	const bye = `trap 'echo bye' EXIT; `
	term := strconv.Itoa(128 + int(syscall.SIGTERM))
	sub := `(trap 'echo sub' EXIT; kill -TERM $$; echo s2); echo after`
	for _, c := range []struct{ dialect, src, want, status string }{
		{"ksh", bye + sub, "bye", term},
		{"bash", bye + sub, "s2 sub bye", term},
		{"zsh", bye + sub, "s2 sub", term},
		{"dash", bye + sub, "s2 sub", term},
		{"ksh", bye + `x=$(trap 'echo sub >&2' EXIT; kill -TERM $$; echo s2 >&2); echo after`, "bye", term},
		{"ksh", bye + `: | (trap 'echo sub' EXIT; kill -TERM $$; echo s2); echo after`, "bye", term},
		{
			"ksh", bye + `(trap 'echo outer' EXIT; (trap 'echo in' EXIT; kill -TERM $$; echo s3); echo s2); echo after`,
			"bye", term,
		},
		{
			"ksh", bye + `(trap 'echo sub' EXIT; kill -SEGV $$; echo s2); echo after`,
			"", strconv.Itoa(128 + int(syscall.SIGSEGV)),
		},
		{
			"ksh", `trap 'echo T' USR1; ` + bye + `(trap 'echo sub' EXIT; kill -USR1 $$; echo s2); echo "after $?"`,
			"T after " + strconv.Itoa(256+int(syscall.SIGUSR1)) + " bye", "0",
		},
	} {
		out, st, err := presets[c.dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatalf("%s: %s: %v", c.dialect, c.src, err)
		}
		if got := strings.Join(strings.Fields(out), " "); got != c.want || strconv.Itoa(st) != c.status {
			t.Errorf("%s: %s\n got %q at %d\nwant %q at %s", c.dialect, c.src, got, st, c.want, c.status)
		}
	}
}

// What a copy aims at `$$` is answered before an and-or list goes on, the
// handler showing the copy's status, and a fatal one ends the shell before the
// list's next command (#7358). Where it waited for the next statement the
// handler ran after the rest of the list, and `a` was written by a shell
// already dying of TERM.
//
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// against bash 5.3.20, bash 3.2.57, zsh 5.9.2 and dash, which agree on every
// row. ksh93u+ 2012-08-01 runs the `( … )` in its own process, which ends
// there at 256 + N without running the list on — this shell's ksh answers so
// already (TestASubshellEndsOnATrappedSignalToTheShellInOneDialect) — and
// agrees on the pipeline and the TERM rows.
func TestACopysSignalToTheShellIsAnsweredBeforeTheListGoesOn(t *testing.T) {
	const trap = `trap 'echo U' USR1; `
	term := strconv.Itoa(128 + int(syscall.SIGTERM))
	for _, dialect := range []string{"bash", "zsh", "dash", "ksh"} {
		for _, c := range []struct{ src, want, status string }{
			{trap + `{ kill -USR1 $$; } | /bin/cat && echo a`, "U a", "0"},
			{`( kill $$ ) && echo a`, "", term},
			{`{ kill $$; } | /bin/cat && echo a`, "", term},
		} {
			out, st, err := presets[dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("%s: %s: %v", dialect, c.src, err)
			}
			if got := strings.Join(strings.Fields(out), " "); got != c.want || strconv.Itoa(st) != c.status {
				t.Errorf("%s: %s\n got %q at %d\nwant %q at %s", dialect, c.src, got, st, c.want, c.status)
			}
		}
	}
	for _, dialect := range []string{"bash", "zsh", "dash"} {
		for _, c := range []struct{ src, want string }{
			{trap + `( kill -USR1 $$ ) && echo a`, "U a"},
			{`trap 'echo U $?' USR1; false; ( kill -USR1 $$; exit 3 ) || echo a$?`, "U 3 a3"},
		} {
			out, st, err := presets[dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("%s: %s: %v", dialect, c.src, err)
			}
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("%s: %s\n got %q at %d\nwant %q at 0", dialect, c.src, got, st, c.want)
			}
		}
	}
}

// What a command substitution's own `kill` aims at `$$` is answered as the
// simple command that held it ends, inside a group's or an `eval`'s
// redirection, the handler in zsh showing the status from before the command
// (#7361). Where it waited for the next statement the handler wrote outside
// the redirection, and zsh's was shown the substitution's status at the
// and-or list's step.
//
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// against bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01 and dash,
// which agree on the redirection rows. The control is the command's own
// redirection, which bash, ksh93 and dash have undone before the handler
// runs; zsh runs it as it arrives and writes nothing there (#7224), which is
// not modeled. ksh93's `$( … )` ends at 256 + N where the signal lands
// (TestASubshellEndsOnATrappedSignalToTheShellInOneDialect), so its status
// row reads 286 for USR1.
func TestASubstitutionsOwnSignalToTheShellIsAnsweredAsTheCommandEnds(t *testing.T) {
	const trap = `trap 'echo U' USR1; `
	const toStderr = `trap 'echo U >&2' USR1; `
	const shown = `trap 'echo U $?' USR1; false; `
	killed := strconv.Itoa(256 + int(syscall.SIGUSR1))
	status := shown + `x=$(kill -USR1 $$; exit 3) || echo a$?`
	type row struct{ src, want string }
	every := []row{
		{toStderr + `{ x=$(kill -USR1 $$); } 2>/dev/null; echo b`, "b"},
		{toStderr + "{ x=`kill -USR1 $$`; } 2>/dev/null; echo b", "b"},
		{trap + `eval 'x=$(kill -USR1 $$)' >/dev/null; echo b`, "b"},
	}
	rows := map[string][]row{
		"bash": {{status, "U 3 a3"}, {trap + `x=$(kill -USR1 $$) >/dev/null; echo b`, "U b"}},
		"dash": {{status, "U 3 a3"}, {trap + `x=$(kill -USR1 $$) >/dev/null; echo b`, "U b"}},
		"zsh":  {{status, "U 1 a3"}},
		"ksh": {
			{status, "U " + killed + " a" + killed},
			{trap + `x=$(kill -USR1 $$) >/dev/null; echo b`, "U b"},
		},
	}
	for dialect, own := range rows {
		for _, c := range append(own, every...) {
			out, st, err := presets[dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("%s: %s: %v", dialect, c.src, err)
			}
			if got := strings.Join(strings.Fields(out), " "); got != c.want || st != 0 {
				t.Errorf("%s: %s\n got %q at %d\nwant %q at 0", dialect, c.src, got, st, c.want)
			}
		}
	}
}

// A signal from another process that lands while an and-or list's left side
// runs is answered before the list goes on, the handler showing the left
// side's status (#7362). Where it waited for the next statement the handler
// ran after the rest of the list: a, U for the first row.
//
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// against bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01 and dash,
// which agree on every row; `/bin/kill -USR1 $$ && echo a` is U, a in all five
// too. The sender lingers a tenth of a second before it exits, so the arrival
// has reached the shell by the time the left side ends. The fifo row's sender
// is a background job that signals once the left side asks it to, and the left
// side ends once it has. The last two rows are controls: a left side ended by
// `;`, or an `if` condition, was answered at the next statement already.
//
// Each row is this test binary as the shell, through the front end and in a
// process group of its own, because the signal is a real one on the shell's
// process: sent to the test binary, it would reach whatever an earlier test
// left listening there.
func TestASignalFromAnotherProcessIsAnsweredBeforeTheListGoesOn(t *testing.T) {
	const trap = `trap 'echo U' USR1; `
	const shown = `trap 'echo U $?' USR1; false; `
	send := func(status string) string {
		return `/bin/sh -c 'kill -USR1 $1; /bin/sleep 0.1; exit $2' sh $$ ` + status
	}
	for _, dialect := range []string{"bash", "zsh", "dash", "ksh"} {
		handshake := `d='` + t.TempDir() + `'; /usr/bin/mkfifo "$d/go" "$d/sent"; ` +
			`/bin/sh -c 'read x <"$2/go"; kill -USR1 $1; echo >"$2/sent"' sh $$ "$d" & ` +
			`/bin/sh -c 'echo >"$1/go"; read x <"$1/sent"; /bin/sleep 0.1; exit 3' sh "$d"`
		for _, c := range []struct{ src, want string }{
			{trap + send("0") + ` && echo a`, "U a"},
			{trap + send("0") + ` && echo a && echo b`, "U a b"},
			{shown + send("3") + ` || echo a$?`, "U 3 a3"},
			{shown + send("0") + ` && echo a$?`, "U 0 a0"},
			{trap + `! ` + send("0") + ` || echo a`, "U a"},
			{trap + `{ ` + send("0") + `; } && echo a`, "U a"},
			{trap + send("0") + ` | /bin/cat && echo a`, "U a"},
			{trap + `f() { ` + send("0") + ` && echo a; }; f`, "U a"},
			{trap + `x=$(` + send("0") + `) && echo a`, "U a"},
			{shown + handshake + ` || echo a$?; wait`, "U 3 a3"},
			{trap + send("0") + `; echo a`, "U a"},
			{trap + `if ` + send("0") + `; then echo a; fi`, "U a"},
		} {
			ws, out := endsOnItsOwn(t, dialect, c.src)
			if got := strings.Join(strings.Fields(out), " "); got != c.want || ws.Signaled() || ws.ExitStatus() != 0 {
				t.Errorf("%s: %s\n got %q, ended %v\nwant %q at 0", dialect, c.src, got, ws, c.want)
			}
		}
	}
}

// A signal from another process that lands while a pipeline runs is answered
// as the pipeline ends, the handler showing the status it left — zsh's too,
// where a child was waited for (#7364) — and before a `!` inverts it, except
// for a negated command of one in bash, whose handler sees the inverted status
// (#7365): interp.Semantics.SignalHandlerSeesTheNegatedStatus.
//
// Measured 2026-11-02 under `-c`, each shell in a process group of its own,
// against bash 5.3.20, bash 3.2.57, zsh 5.9.2, ksh93u+ 2012-08-01 and dash.
// The sender lingers a tenth of a second before it exits, as in the test
// above, so the arrival has reached the shell by the time the command ends;
// `/bin/kill -USR1 $$; echo b$?`, U 0 in all five, is that row with no
// linger, and the runtime may hand the signal over after the next command
// here. The rows of a negated command of one split bash, U 0 after `false` and U 1
// after `true`, from zsh, ksh93 and dash, U 3 and U 0; the rest agree in all
// five but the last two. There zsh's handler sees the earlier status, inside
// its own builtin — the shell's own `kill`, U 1 where the others show 0, and a
// `wait` the arrival interrupts, U 1 where bash and dash show 128 + USR1 and
// ksh93 shows 1 and its `wait` returns 1 — which is
// interp.Semantics.SignalHandlerSeesEarlierStatus and holds. The handler of
// the last row prints whether `$?` is past 128, the numbers being the
// platform's.
func TestASignalFromAnotherProcessIsAnsweredAsThePipelineEnds(t *testing.T) {
	const shown = `trap 'echo U $?' USR1; false; `
	send := func(status string) string {
		return `/bin/sh -c 'kill -USR1 $1; /bin/sleep 0.1; exit $2' sh $$ ` + status
	}
	const interrupted = `trap 'echo U $(($? > 128))' USR1; ` +
		`/bin/sh -c '/bin/sleep 0.2; kill -USR1 $1' sh $$ >/dev/null 2>&1 & ` +
		`/bin/sleep 3 >/dev/null 2>&1 & false; wait $!; echo b$(($? > 128))`
	for _, dialect := range []string{"bash", "zsh", "dash", "ksh"} {
		negated, ownKill, waited := "U 3", "U 0 b0", "U 1 b1"
		switch dialect {
		case "bash":
			negated = "U 0"
		case "zsh":
			ownKill, waited = "U 1 b0", "U 0 b1"
		case "ksh":
			waited = "U 0 b0"
		}
		afterTrue := "U 0 b1"
		if dialect == "bash" {
			afterTrue = "U 1 b1"
		}
		// The last command's row ends the shell at its status. ksh93 runs that
		// command in place of itself and answers nothing; this shell's ksh
		// answers as the others do.
		for _, c := range []struct {
			src, want string
			st        int
		}{
			{shown + send("3") + `; echo b$?`, "U 3 b3", 0},
			{shown + `if ` + send("3") + `; then :; else echo e$?; fi`, "U 3 e3", 0},
			{shown + `while ` + send("0") + `; do echo w$?; break; done`, "U 0 w0", 0},
			{shown + `f() { ` + send("3") + `; }; f; echo b$?`, "U 3 b3", 0},
			{shown + send("3"), "U 3", 3},
			{shown + `if ` + send("3") + `; then :; fi; echo b$?`, "U 3 b0", 0},
			{shown + `{ ` + send("3") + `; } >/dev/null; echo a$?`, "a3", 0},
			{shown + `! { ` + send("3") + `; }; echo b$?`, "U 3 b0", 0},
			{shown + `! ` + send("3") + ` | /bin/cat; echo b$?`, "U 0 b1", 0},
			{shown + `! ` + send("3") + `; echo b$?`, negated + " b0", 0},
			{shown + `! ` + send("3") + ` && echo a$?`, negated + " a0", 0},
			{shown + `! ( ` + send("3") + ` ); echo b$?`, negated + " b0", 0},
			{shown + `f() { ! ` + send("3") + `; }; f; echo b$?`, negated + " b0", 0},
			{`trap 'echo U $?' USR1; true; ! ` + send("0") + `; echo b$?`, afterTrue, 0},
			{shown + `kill -USR1 $$; echo b$?`, ownKill, 0},
			{interrupted, waited, 0},
		} {
			ws, out := endsOnItsOwn(t, dialect, c.src)
			if got := strings.Join(strings.Fields(out), " "); got != c.want || ws.Signaled() || ws.ExitStatus() != c.st {
				t.Errorf("%s: %s\n got %q, ended %v\nwant %q at %d", dialect, c.src, got, ws, c.want, c.st)
			}
		}
	}
}
