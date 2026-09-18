// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/repl"
)

// A command substitution whose body will not parse, **typed at a prompt**
// (#3300).
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C TERM=dumb` with a
// scratch HOME and ZDOTDIR so no startup file of anybody's is read, the lines
// fed one at a time down a pipe with a gap between them, BusyBox v1.37.0 in
// the digest-pinned Alpine image internal/oracle reaches, under `--init`:
//
//	                     output            status  prompt survived
//	bash 5.3.20          one, two, three   0       yes
//	that binary as `sh`  one, two, three   0       yes
//	bash 3.2.57          one, two, three   0       yes
//	zsh 5.9.2            one, two, three   0       yes
//	ksh93u+ 2012-08-01   one, two, three   0       yes
//	dash 0.5.12          one, two, three   0       yes
//	BusyBox ash 1.37.0   one, two, three   0       yes
//
// **Unanimous, dash included**, so this asks no axis and adds none. The dash
// column is worth a sentence because #3300 filed it as an exit: fed the whole
// program at once, dash reports the failure, discards the input it had already
// buffered and reaches end of file, exiting with the status the failure left.
// The prompt was never lost — it draws the next one, which the bytes show —
// and with the lines fed one at a time every line after runs. The same is true
// through a pseudo-terminal, which is the route a person is on.
//
// Ours ended the session, in all five dialects and on every route.
//
// **The assertion is the line after**, not the status and not the diagnostic:
// a session that reported the failure, drew a prompt and then died on the next
// line looks identical to a live one until something is asked to run. That is
// exactly what the pty transcript of the subshell shape showed before this —
// the prompt came back and `echo` was echoed, and nothing ran it.
func TestASubstitutionThatWillNotParseCostsTheTypedLineAndNotTheSession(t *testing.T) {
	const after = "one\ntwo\n"
	for _, c := range []struct {
		name, line string
		want       map[string]string
	}{
		{
			// The issue's own line.
			name: "at the top level",
			line: "v=$(echo hi; for)",
		},
		{
			// The shape #3299 widened this to: the failure escapes the
			// subshell in five of the six presets, and the typed line *ends*
			// at the subshell, so the stop it recorded is still in the shared
			// box when the line is over.
			name: "inside a subshell",
			line: "( v=$(echo hi; for) )",
		},
		{
			// A pipeline element, which carries a status of its own — the
			// case scriptStop.status exists for.
			name: "in a pipeline element",
			line: "v=$(echo hi; for) | cat",
		},
		{
			// A here-document body, which is the boundary this fix
			// deliberately does not let catch the stop — the *command* is
			// given up there and how far the stop reaches is a question of
			// its own. At a prompt the two halves agree: measured on bash
			// 5.3.20, zsh 5.9.2, ksh93u+ and dash, none of them runs `cat`
			// and all of them draw the next prompt. Ours ran `cat` with a
			// truncated body and then lost the session on the next line.
			name: "in a here-document body",
			line: "cat <<END\nbefore $(echo hi; for) after\nEND",
		},
		{
			// The rest of the *typed line* goes with it, because the line is
			// the unit — measured at a prompt on bash 5.3.20, zsh 5.9.2 and
			// dash, each of which writes `one` and `two` and never `x`. ksh93
			// writes `x`, and so does the one preset that answers
			// SubstitutionParseErrorEscapesASubshell `No`.
			name: "the rest of the line goes with it",
			line: "( v=$(echo hi; for) ); printf 'x\\n'",
			want: map[string]string{"ksh": "one\nx\ntwo\n"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "printf 'one\\n'\n" + c.line + "\nprintf 'two\\n'\n"
			for name, p := range presets {
				t.Run(name, func(t *testing.T) {
					want := after
					if w, ok := c.want[name]; ok {
						want = w
					}
					out, errs := session(t, p, src)
					if out != want {
						t.Errorf("out = %q, want %q — the session should have survived the line", out, want)
					}
					if errs == "" {
						t.Errorf("nothing was reported; the failure has to be visible as well as survivable")
					}
				})
			}
		})
	}
}

// The half that keeps this from making a session nobody can leave.
//
// A request to *stop* is not an error, and the boundary this fix annotates
// deliberately lets one through. Measured at a prompt in every column: `exit`,
// `exit 7` and errexit firing each end the session.
func TestARequestToStopStillEndsTheSessionAfterTheSubstitutionFix(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		{"exit", "exit", "one\n"},
		{"exit with a status", "exit 7", "one\n"},
		{"exit inside a substitution's own body", "v=$(exit 7; echo no)", "one\ntwo\n"},
		// The control that says the rows above are not passing because
		// nothing runs at all.
		{"a plain line", "printf 'mid\\n'", "one\nmid\ntwo\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "printf 'one\\n'\n" + c.line + "\nprintf 'two\\n'\n"
			for name, p := range presets {
				t.Run(name, func(t *testing.T) {
					if out, _ := session(t, p, src); out != c.want {
						t.Errorf("out = %q, want %q", out, c.want)
					}
				})
			}
		})
	}
}

// And the same failure in a **script** is untouched, which is the control that
// says this moved a boundary rather than the fatality.
//
// Every row here is dialect/substfatal_test.go's, re-asserted from the other
// side: the status is the dialect's syntax status and nothing after the line
// runs. A fix that had made the failure survivable everywhere would pass every
// row of the test above and fail every row of this one.
func TestTheSameFailureInAScriptIsUnchanged(t *testing.T) {
	const src = "printf 'start\\n'\nv=$(echo hi; for)\nprintf 'after st=%s\\n' \"$?\"\n"
	for _, c := range []struct {
		preset string
		status int
	}{
		{"bash", 2}, {"zsh", 1}, {"ksh", 3}, {"dash", 2}, {"ash", 2}, {"posix", 2},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, _, st := splitRun(t, presets[c.preset], src)
			if out != "start\n" || st != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, st, "start\n", c.status)
			}
		})
	}
}

// session runs the source through the prompt loop, with no terminal, and
// answers standard output and the error stream separately.
//
// Interactive rather than a script runner, because the boundary under test is
// the one a session draws: [interp.Runner.GiveUpTheLine] is reached from the
// prompt loop and from nowhere a script goes through. The reader is not a
// descriptor, which sends [repl.Shell.Run] to the loop that reads lines and
// draws prompts without an editor — the same loop, and the one the `-i`
// columns above were measured on.
func session(t *testing.T, p dialecttest.Preset, src string) (out, errs string) {
	t.Helper()
	var o, e strings.Builder
	r := p.Runner(dialecttest.Base{Stdout: &o, Stderr: &e, Interactive: true})
	s := repl.Shell{
		Runner:  r,
		Dialect: p.Dialect(),
		Name:    p.Name,
		In:      strings.NewReader(src),
		Out:     &e,
		Err:     &e,
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("the session refused the input: %v", err)
	}
	return o.String(), e.String()
}

// The redirection boundary, which is the one place the annotation stops short
// — and the two halves of what it does there, kept apart.
//
// A here-document body holding such a substitution costs the **command** in
// every column: measured 2026-09-16 from a script, none of bash 5.3.20, zsh
// 5.9.2, ksh93u+ or dash runs `cat`, and bash 3.2.57 is the third answer as it
// is everywhere in this file. That half is what these rows hold: `cat` runs in
// no dialect, whatever becomes of the stop.
//
// **How far the stop then reaches is the second half and the columns split on
// it** — bash 5.3.20 carries the script on at 1 and ksh93u+ at 3, where zsh
// 5.9.2 ends it at 1 and dash and BusyBox ash at 2. That was filed as a gap
// and is now
// Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell, whose panel
// and whose statuses are in substheredocgiveup_test.go. Kept apart here so
// that this file stays about the annotation the prompt boundary reads: giving
// up neither would run `cat` with a body the substitution left half-expanded,
// which is output no column produces.
func TestAHereDocumentBodyIsTheOneBoundaryThatKeepsItsAnswer(t *testing.T) {
	const src = "printf 'start\\n'\ncat <<END\nbefore $(echo hi; for) after\nEND\nprintf 'after st=%s\\n' \"$?\"\n"
	for _, preset := range []string{"bash", "zsh", "ksh", "dash", "ash", "posix"} {
		t.Run(preset, func(t *testing.T) {
			out, errs, _ := splitRun(t, presets[preset], src)
			if !strings.HasPrefix(out, "start\n") || strings.Contains(out, "before") {
				t.Errorf("wrote %q, want it to begin `start` and never hold the body — `cat` must not run", out)
			}
			if errs == "" {
				t.Errorf("nothing was reported")
			}
		})
	}
}

// The seam #3300 asked to be measured before the annotation was taken: `.` and
// `eval`, which catch the same kind.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh`
// with stdin from /dev/null, on a file whose second line is the substitution
// and whose caller prints `after` and `$?`:
//
//	                     .                     eval
//	bash 5.3.20          ends the script (1)   ends the script (1)
//	bash 3.2.57          carries on (0)        carries on (0)
//	zsh 5.9.2            after dot st=126      after eval st=1
//	ksh93u+ 2012-08-01   after dot st=3        after eval st=3
//	dash 0.5.12          ends the script (2)   ends the script (2)
//
// So the two columns that make borrowed text a boundary catch this exactly as
// they catch every other error there, and the two that do not, do not. That is
// [interp.Semantics.FatalErrorEndsBorrowedTextOnly] answered as it already is,
// and the annotation is what lets it be asked at all: as a request to stop the
// failure went straight past the boundary and ended the script in all six
// presets. The statuses fall out of the answers already recorded — zsh's 126
// is Diagnostics.SourcedFatalStatus and ksh's is the syntax status the failure
// carries — so nothing here is a number chosen to fit.
//
// bash's and dash's rows differ from their references only in the status the
// shell exits with, which is what that failure has always reported and is not
// this change's: #3296 is the sentence, and the exit status of a script a
// sourced file's failure ended is filed with it.
func TestTheBorrowedTextBoundaryCatchesItWhereTheDialectSaysSo(t *testing.T) {
	for _, c := range []struct {
		name, src string
		want      map[string]string
	}{
		{
			name: "a sourced file",
			src: "printf 'start\\n'\n. ./inner\n" +
				"printf 'after st=%s\\n' \"$?\"\n",
			want: map[string]string{
				"zsh": "start\ninner-start\nafter st=126\n",
				"ksh": "start\ninner-start\nafter st=3\n",
			},
		},
		{
			name: "an eval argument",
			src: "printf 'start\\n'\neval 'printf \"inner-start\\n\"; v=$(echo hi; for); printf \"inner-after\\n\"'\n" +
				"printf 'after st=%s\\n' \"$?\"\n",
			want: map[string]string{
				"zsh": "start\ninner-start\nafter st=1\n",
				"ksh": "start\ninner-start\nafter st=3\n",
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			const inner = "printf 'inner-start\\n'\nv=$(echo hi; for)\nprintf 'inner-after\\n'\n"
			if err := os.WriteFile(filepath.Join(dir, "inner"), []byte(inner), 0o600); err != nil {
				t.Fatal(err)
			}
			for name, p := range presets {
				t.Run(name, func(t *testing.T) {
					// The columns whose borrowed text is not a boundary: the
					// script ends where it stands, so nothing after runs.
					want := "start\ninner-start\n"
					if w, ok := c.want[name]; ok {
						want = w
					}
					var o, e strings.Builder
					r := p.Runner(dialecttest.Base{Stdout: &o, Stderr: &e, Dir: dir})
					if _, err := r.Run(t.Context(), p.Parse(t, c.src)); err != nil {
						t.Fatalf("run: %v", err)
					}
					if got := o.String(); got != want {
						t.Errorf("out = %q, want %q", got, want)
					}
					if strings.Contains(o.String(), "inner-after") {
						t.Errorf("the borrowed text ran on past the failure: %q", o.String())
					}
				})
			}
		})
	}
}
