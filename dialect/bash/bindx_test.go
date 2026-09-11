// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// `bind -x`, measured against bash 5.3.15 on 2026-09-11 through a
// pseudo-terminal, driving real keys at a real prompt. Every want below is
// what that binary did.

// bindxRunner runs some source and hands back the Runner it ran in, so a test
// can then press the key the way the front end does. bindRun cannot be used
// for those: it keeps the Runner to itself, and the binding table and the line
// are both state.
func bindxRunner(t *testing.T, src string) (*interp.Runner, *strings.Builder) {
	t.Helper()
	var out strings.Builder
	r := preset.Runner(dialecttest.Base{Stdout: &out, Stderr: &out, Dir: t.TempDir()})
	r.Interactive = true
	if _, err := r.Run(t.Context(), preset.Parse(t, src)); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return r, &out
}

// press is what the front end does with a key bound to a shell command: it
// finds the command on the key and hands the line to the dialect.
//
// Through KeyBindings rather than with the command text written out again,
// because the path from `bind -x` to a running command is the thing under
// test and half of it is that table.
func press(t *testing.T, r *interp.Runner, seq string, in repl.Line) (repl.Line, bool) {
	t.Helper()
	bound, ok := bash.KeyBindings(r)[seq]
	if !ok {
		t.Fatalf("no binding on %q: the table is %v", seq, bash.KeyBindings(r))
	}
	if bound.Function == "" {
		t.Fatalf("%q reached the editor as %v, want a shell command on it", seq, bound)
	}
	return bash.RunWidget(r, context.Background(), bound.Function, in)
}

// The round trip: the line goes out, the command reads it, and what the
// command left comes back.
//
// Measured with `abcdef` typed and the cursor at the end: a bound command
// reads READLINE_LINE=abcdef and READLINE_POINT=6.
func TestABoundCommandReadsTheLineAndThePoint(t *testing.T) {
	r, out := bindxRunner(t, `bind -x '"\C-t": printf "[%s][%s]" "$READLINE_LINE" "$READLINE_POINT"'`)
	line, ok := press(t, r, "\x14", repl.Line{Buffer: "abcdef", Cursor: 6})
	if !ok {
		t.Fatal("the command did not run")
	}
	if want := "[abcdef][6]"; out.String() != want {
		t.Errorf("the command saw %q, want %q", out.String(), want)
	}
	// And a command that read the line and left it alone hands back the line
	// it was given, rather than an empty one.
	if want := (repl.Line{Buffer: "abcdef", Cursor: 6}); line != want {
		t.Errorf("line back = %v, want %v unchanged", line, want)
	}
}

// The point is in characters and not in bytes.
//
// Measured with `caféx` typed — five characters, six bytes — and the cursor at
// the end: READLINE_POINT reads 5.
func TestThePointCountsCharactersRatherThanBytes(t *testing.T) {
	r, out := bindxRunner(t, `bind -x '"\C-t": printf "%s" "$READLINE_POINT"'`)
	if _, ok := press(t, r, "\x14", repl.Line{Buffer: "caféx", Cursor: 5}); !ok {
		t.Fatal("the command did not run")
	}
	if out.String() != "5" {
		t.Errorf("READLINE_POINT = %q on a five-character six-byte line, want %q", out.String(), "5")
	}
}

// A command that assigns either parameter changes the line the editor draws.
//
// Measured: `READLINE_LINE=NEWTEXT; READLINE_POINT=3` on a line reading
// `abcdef` leaves the editor drawing NEWTEXT with the cursor three characters
// in, and pressing return then submits NEWTEXT.
func TestACommandCanRewriteTheLineAndMoveThePoint(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		in, want      repl.Line
	}{
		{
			"both", `READLINE_LINE=NEWTEXT; READLINE_POINT=3`,
			repl.Line{Buffer: "abcdef", Cursor: 6},
			repl.Line{Buffer: "NEWTEXT", Cursor: 3},
		},
		{
			// Measured: a command that sets only READLINE_POINT redraws with
			// the cursor moved, and typing after it inserts at the new place.
			"the point alone", `READLINE_POINT=2`,
			repl.Line{Buffer: "abcdef", Cursor: 6},
			repl.Line{Buffer: "abcdef", Cursor: 2},
		},
		{
			// Measured: setting only READLINE_LINE to something shorter puts
			// the cursor at the end of what is left rather than off the end.
			// That is the clamp, and it is why the clamp is on the read.
			"the line alone, and shorter", `READLINE_LINE=ZZ`,
			repl.Line{Buffer: "abcdef", Cursor: 6},
			repl.Line{Buffer: "ZZ", Cursor: 2},
		},
		{
			// Out of range is brought back rather than refused, which is what
			// the editor does with a cursor either way — see repl.Line.
			"a point past the end", `READLINE_POINT=999`,
			repl.Line{Buffer: "abc", Cursor: 0},
			repl.Line{Buffer: "abc", Cursor: 3},
		},
		{
			"a point before the start", `READLINE_POINT=-5`,
			repl.Line{Buffer: "abc", Cursor: 3},
			repl.Line{Buffer: "abc", Cursor: 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := bindxRunner(t, `bind -x '"\C-t": `+tc.command+`'`)
			got, ok := press(t, r, "\x14", tc.in)
			if !ok {
				t.Fatal("the command did not run")
			}
			if got != tc.want {
				t.Errorf("line back = %v, want %v", got, tc.want)
			}
		})
	}
}

// The command's status goes in and does not come out.
//
// Measured: a key bound to a command that returns 42 leaves `echo $?` at the
// next prompt saying 0. So `$?` is what the command *sees*, and what it leaves
// is not what the next command reads.
func TestTheStatusGoesInAndDoesNotComeOut(t *testing.T) {
	r, out := bindxRunner(t,
		`f() { printf "[%s]" "$?"; return 42; }`+"\n"+
			`bind -x '"\C-t": f'`+"\n"+
			// Last, so that the status the key is pressed on is this one and
			// not `bind`'s own success.
			"false\n")
	if _, ok := press(t, r, "\x14", repl.Line{}); !ok {
		t.Fatal("the command did not run")
	}
	if want := "[1]"; out.String() != want {
		t.Errorf("the command saw $? = %q, want %q — the status the last command left", out.String(), want)
	}
	if got := r.ExitStatus(); got != 1 {
		t.Errorf("$? after the key is %d, want 1 — the command's 42 must not reach it", got)
	}
}

// The three parameters exist only while the command runs.
//
// Measured at the next prompt: `${READLINE_LINE-UNSET}` is UNSET, and so is
// each of the others. A script that is not running a bound command must find
// them unset rather than empty, which is the difference `-` tests for.
func TestTheParametersDoNotOutliveTheCall(t *testing.T) {
	r, out := bindxRunner(t, `bind -x '"\C-t": :'`)
	if _, ok := press(t, r, "\x14", repl.Line{Buffer: "abc", Cursor: 1}); !ok {
		t.Fatal("the command did not run")
	}
	out.Reset()
	src := `printf "[%s][%s]" "${READLINE_LINE-UNSET}" "${READLINE_POINT-UNSET}"`
	if _, err := r.Run(t.Context(), preset.Parse(t, src)); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	if want := "[UNSET][UNSET]"; out.String() != want {
		t.Errorf("after the call the parameters are %q, want %q", out.String(), want)
	}
}

// It runs in the current shell, not a subshell.
//
// Measured: a key bound to `MARKER=yes` leaves `$MARKER` set at the next
// prompt. A command that changes the shell means it.
func TestTheCommandRunsInTheCurrentShell(t *testing.T) {
	r, _ := bindxRunner(t, `bind -x '"\C-t": MARKER=yes'`)
	if _, ok := press(t, r, "\x14", repl.Line{}); !ok {
		t.Fatal("the command did not run")
	}
	if got, set := r.GetVar("MARKER"); !set || got != "yes" {
		t.Errorf("MARKER = %q (set %v) after the key, want yes — the command runs in this shell", got, set)
	}
}

// A key bound to nothing to run is declined rather than run, and the line is
// handed back exactly as it came.
func TestAnEmptyCommandIsDeclined(t *testing.T) {
	in := repl.Line{Buffer: "abc", Cursor: 2}
	r := preset.Runner(dialecttest.Base{Dir: t.TempDir()})
	got, ok := bash.RunWidget(r, context.Background(), "", in)
	if ok {
		t.Error("an empty command reported that it ran")
	}
	if got != in {
		t.Errorf("line back = %v, want %v unchanged", got, in)
	}
}

// `bind -x` takes only the quoted form, which is measured and not a
// simplification: the key-name form `bind` itself accepts is refused here.
//
//	bash: bind: \C-t:echo u: first non-whitespace character is not `"'
//
// at status 1, and the same for a word with no colon in it at all.
func TestTheCommandFormMustBeQuoted(t *testing.T) {
	for _, src := range []string{`bind -x "\C-t:echo u"`, `bind -x "nocolon"`} {
		out, st := bindRun(t, src)
		if want := "first non-whitespace character is not `\"'"; !strings.Contains(out, want) {
			t.Errorf("%s printed %q, want it to contain %q", src, out, want)
		}
		if st != 1 {
			t.Errorf("%s status = %d, want 1", src, st)
		}
	}
	// And the quoted form is accepted, so the refusals above are about the
	// form and not about `-x` being dead.
	if out, st := bindRun(t, `bind -x '"\C-t": echo hi'`); out != "" || st != 0 {
		t.Errorf("the quoted form printed %q at %d, want silence at 0", out, st)
	}
}

// `bind -X` is its own listing, and its shape is not the one every other
// listing here uses.
//
// Measured: there is no colon between the key and the command, and the command
// is quoted where `bind -p` leaves a function name bare. Sorted by the bytes
// the key sends, and nothing at all where none were made.
func TestTheCommandListingHasItsOwnShape(t *testing.T) {
	out, st := bindRun(t, `bind -x '"\C-t": names'`+"\n"+
		`bind -x '"\C-n": echo hi there; pwd'`+"\n"+`bind -X`)
	want := "\"\\C-n\" \"echo hi there; pwd\"\n\"\\C-t\" \"names\"\n"
	if out != want || st != 0 {
		t.Errorf("bind -X = %q at %d, want %q at 0", out, st, want)
	}
	if out, st := bindRun(t, "bind -X"); out != "" || st != 0 {
		t.Errorf("bind -X with none made = %q at %d, want silence at 0", out, st)
	}
}

// The command text is stored as it stands: not parsed, not looked up, not
// judged.
//
// That is the opposite of the rule for a *function* name, which is checked and
// dropped when unknown so the key goes on doing what it did — and both halves
// are measured. There is nothing to check here: `bind -X` prints back what was
// given, and a command that does not work says so when the key is pressed.
func TestAnUnknownCommandIsStoredWhereAnUnknownFunctionIsNot(t *testing.T) {
	out, _ := bindRun(t, `bind -x '"\C-t": no-such-command --flag'`+"\n"+`bind -X`)
	if want := "\"\\C-t\" \"no-such-command --flag\"\n"; out != want {
		t.Errorf("bind -X = %q, want %q", out, want)
	}
	// The control, from the other rule: an unknown *function* name is not
	// stored at all, so the key is still the editor's own and nothing in the
	// two listings mentions it.
	out, _ = bindRun(t, `bind '"\C-t": no-such-widget'`+"\n"+`bind -X`)
	if out != "" {
		t.Errorf("an unknown function name was stored: bind -X = %q", out)
	}
}

// A key running a command names no function, so the listings and the queries
// that walk function names leave it out.
//
// Measured: `bind -p` has no row for a key `-x` bound, and `bind -q` answers
// `unknown function name` at 1 for the command's text.
func TestACommandIsNotAFunctionName(t *testing.T) {
	out, _ := bindRun(t, `bind -x '"\C-t": f'`+"\n"+`bind -p`)
	if strings.Contains(out, `"\C-t": f`) {
		t.Errorf("bind -p listed a key bound to a command:\n%s", out)
	}
	out, st := bindRun(t, `bind -x '"\C-t": f'`+"\n"+`bind -q f`)
	if want := "bind: `f': unknown function name"; !strings.Contains(out, want) || st != 1 {
		t.Errorf("bind -q f = %q at %d, want %q at 1", out, st, want)
	}
	// And `-s`/`-S`, which list the keys bound to *text*, do not claim it
	// either: a command is neither a function nor a macro.
	if out, _ := bindRun(t, `bind -x '"\C-t": f'`+"\n"+`bind -S`); out != "" {
		t.Errorf("bind -S listed a key bound to a command: %q", out)
	}
}

// One slot per key, measured in both directions: `-r` takes a command off, and
// an ordinary `bind` over the same key replaces it rather than living beside
// it.
func TestOneSlotPerKey(t *testing.T) {
	out, st := bindRun(t, `bind -x '"\C-t": f'`+"\n"+`bind -r "\C-t"`+"\n"+`bind -X`)
	if out != "" || st != 0 {
		t.Errorf("after -r, bind -X = %q at %d, want silence at 0", out, st)
	}
	out, _ = bindRun(t, `bind -x '"\C-t": f'`+"\n"+
		`bind '"\C-t": clear-screen'`+"\n"+`bind -X`+"\n"+`bind -p`)
	if strings.Contains(out, "\"\\C-t\" \"f\"") {
		t.Errorf("the command survived an ordinary bind on the same key:\n%s", out)
	}
	if !strings.Contains(out, `"\C-t": clear-screen`) {
		t.Errorf("the ordinary bind did not take:\n%s", out)
	}
	// And the other way round: `-x` over a key an ordinary `bind` had.
	out, _ = bindRun(t, `bind '"\C-t": clear-screen'`+"\n"+
		`bind -x '"\C-t": f'`+"\n"+`bind -X`+"\n"+`bind -p`)
	if !strings.Contains(out, "\"\\C-t\" \"f\"") {
		t.Errorf("the command did not take over the key:\n%s", out)
	}
	if strings.Contains(out, `"\C-t": clear-screen`) {
		t.Errorf("the function survived a -x on the same key:\n%s", out)
	}
}

// The binding reaches the editor as a Function and not as a Widget, which is
// the seam: repl.Binding.Function carries a name repl does not look inside, and
// what this shell puts on it is the command text itself.
func TestTheCommandReachesTheEditorOnFunction(t *testing.T) {
	r, _ := bindxRunner(t, `bind -x '"\C-x\C-r": echo one; echo two'`)
	got, bound := bash.KeyBindings(r)["\x18\x12"]
	if !bound {
		t.Fatalf("^X^R reached the editor as nothing: %v", bash.KeyBindings(r))
	}
	want := repl.Binding{Function: "echo one; echo two"}
	if got != want {
		t.Errorf("^X^R reached the editor as %v, want %v", got, want)
	}
}
