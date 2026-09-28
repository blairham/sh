// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// `umask` reads its own options, and a lone `-` ends them there as it does
// everywhere else.
//
// Once the dash is eaten there is no operand left, so the builtin is its bare
// form and the bare form **writes the mask**. Before this the dash was
// hard-coded as an operand and went on to be parsed as a mask, so `umask -`
// was silent at 0 — a mask nobody could read, reported as success.
//
// Measured 2026-09-28 against zsh 5.9.2 from a script file under `env -i`:
// `umask -` writes `022` where `umask` alone writes `022`, and `umask - 022`
// sets the mask, so what follows the dash really is the operand.
//
// Graded here rather than beside the other builtins of #5040 because this is
// the only one of them that needs a seam — a runner with no SetUmask answers
// "this shell was not given a umask to read or change" whatever the dash does,
// which is a null that would pass a test written the other way round.
func TestALoneDashEndsTheUmaskOptions(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		wantMask        int
	}{
		{"the dash is eaten and the bare form writes the mask", `umask -`, "022", 0o022},
		{"as the bare form itself does", `umask`, "022", 0o022},
		{"and an operand behind the dash is still the operand", `umask - 077`, "", 0o077},
		{"while the operand alone still sets it", `umask 077`, "", 0o077},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The hook is asked for the old mask once, at startup, and the
			// shell keeps the mask itself from then on — so what to assert
			// on afterwards is r.umask and what the builtin wrote, never the
			// variable the hook last saw. That is 0, correctly, because
			// ensureUmask sets the kernel's to 0 and masks by hand.
			mask := 0o022
			var buf bytes.Buffer
			sem := CoreSemantics()
			sem.LoneDashIsAnOption = Yes
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf,
				Semantics: &sem, Diagnostics: &Diagnostics{},
				Dir: t.TempDir(), Name: "testsh",
				SetUmask: func(m int) (int, error) { old := mask; mask = m; return old, nil },
			})
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatalf("run: %v", rerr)
			}
			if tc.want == "" {
				if strings.TrimSpace(buf.String()) != "" {
					t.Errorf("out = %q, want nothing written", buf.String())
				}
			} else if !strings.Contains(buf.String(), tc.want) {
				t.Errorf("out = %q, want it to contain %q", buf.String(), tc.want)
			}
			if r.umask != tc.wantMask {
				t.Errorf("the shell's mask = %#o, want %#o", r.umask, tc.wantMask)
			}
		})
	}
	// And a dialect that has not answered leaves the dash where it was, which
	// is a mask it cannot read — the behavior every column but one has.
	t.Run("a dialect that reads no lone dash keeps it as the operand", func(t *testing.T) {
		mask := 0o022
		var buf bytes.Buffer
		sem := CoreSemantics()
		sem.LoneDashIsAnOption = No
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf,
			Semantics: &sem, Diagnostics: &Diagnostics{},
			Dir: t.TempDir(), Name: "testsh",
			SetUmask: func(m int) (int, error) { old := mask; mask = m; return old, nil },
		})
		f, err := syntax.Parse(`umask -`, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if _, rerr := r.Run(context.Background(), f); rerr != nil {
			t.Fatalf("run: %v", rerr)
		}
		if strings.Contains(buf.String(), "022") {
			t.Errorf("out = %q, want the dash read as a mask rather than eaten", buf.String())
		}
	})
}

// `type` answers a lone `-` on the lone-dash axis, and not on the one about
// `--`.
//
// The two are different words and different questions, and they agree in the
// dialect this was written for — zsh answers both `Yes` — which is exactly why
// a grid built from that dialect cannot tell them apart. Mutation said so:
// deleting the branch that answers the dash killed nothing, because the dash
// then fell through to the `--` question and on to the shared reader, which
// got the right answer for the wrong reason.
//
// The rows that part them hold the dash fixed and move the *other* axis.
func TestTypeAnswersALoneDashOnItsOwnAxis(t *testing.T) {
	run := func(t *testing.T, lone, dashdash Answer) (string, int) {
		t.Helper()
		var buf bytes.Buffer
		sem := CoreSemantics()
		sem.LoneDashIsAnOption = lone
		sem.TypeEndsOptionsWithDashDash = dashdash
		sem.TypeOptions = "afpsSw"
		// Answered so the run says nothing about it: `type` looks a name up
		// on PATH, and an unanswered axis on that road would put its own
		// complaint in the output these rows read.
		sem.EmptyPathIsTheCurrentDirectory = No
		r := newTestRunner(t, &Runner{
			Stdout: &buf, Stderr: &buf,
			Semantics: &sem, Diagnostics: &Diagnostics{},
			Dir: t.TempDir(), Name: "testsh",
		})
		f, err := syntax.Parse(`type - -a echo`, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		return buf.String(), st
	}
	// A dialect that ends nothing at `--` still eats a lone dash, because the
	// dash was never the `--` question. The dash is gone, so `-a` is a name.
	t.Run("the dash is eaten although `--` ends nothing here", func(t *testing.T) {
		out, _ := run(t, Yes, No)
		if strings.Contains(out, "type: -: not found") {
			t.Errorf("out = %q, want the dash eaten rather than looked up", out)
		}
		if !strings.Contains(out, "type: -a: not found") {
			t.Errorf("out = %q, want `-a` read as a name", out)
		}
	})
	// And the other way: a dialect that ends the options at `--` still leaves
	// a lone dash alone when the lone-dash axis says so.
	t.Run("and left alone although `--` does end the options here", func(t *testing.T) {
		out, _ := run(t, No, Yes)
		if !strings.Contains(out, "type: -: not found") {
			t.Errorf("out = %q, want the dash looked up as a name", out)
		}
	})
	// **And the refusal names the axis it actually asked.** Unanswered, the
	// complaint is about the lone dash and not about `type --`, which is a
	// word the script never wrote.
	t.Run("an unanswered lone dash names the lone dash", func(t *testing.T) {
		out, st := run(t, Unspecified, Yes)
		if !strings.Contains(out, "a lone `-` given to a builtin") {
			t.Errorf("out = %q, want the lone-dash axis named", out)
		}
		if strings.Contains(out, "`type --`") {
			t.Errorf("out = %q, want the `--` axis NOT named", out)
		}
		if st != 2 {
			t.Errorf("status = %d, want 2", st)
		}
	})
}
