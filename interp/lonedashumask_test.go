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
