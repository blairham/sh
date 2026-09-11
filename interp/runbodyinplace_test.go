// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A body run in place runs in the frame that is already open, where a call
// opens one of its own.
//
// The seam is Runner.RunFunctionBodyInPlace, and this names it rather than a
// shell: a builtin that has *replaced* the function it is running inside has
// to carry on in the body it put there, and a nested call would leave a frame
// on the stack that is there on the first call and gone on every one after.
//
// Both halves in one test, on one runner, because the pair is the assertion:
// the same builtin either calls or runs in place, and everything else about
// the two paths is identical.
func TestABodyRunInPlaceOpensNoFrameOfItsOwn(t *testing.T) {
	// The replacement body: it reports how deep it is and what its
	// positional parameters are, then declares a local so that the scope can
	// be checked to have unwound.
	const loaded = `zzdepth; echo "args=<$*>"; local loc=inner; return 7`

	install := func(inPlace bool) func(*Runner) {
		return func(r *Runner) {
			// The depth probe. A frame with no name is the script's own, so
			// this counts the named ones the way a call-stack parameter does.
			r.Register("zzdepth", func(rr *Runner, _ context.Context, _ []string) int {
				named := 0
				for _, f := range rr.CallStack() {
					if f.Name != "" {
						named++
					}
				}
				_, _ = fmt.Fprintf(rr.Stdout, "depth=%d\n", named)
				return 0
			})
			// The stand-in for a resolution: it redefines the function it is
			// running inside and then continues into the new body.
			r.Register("zzresolve", func(rr *Runner, ctx context.Context, _ []string) int {
				name := ""
				for _, f := range rr.CallStack() {
					if f.Name != "" {
						name = f.Name
						break
					}
				}
				if !rr.DefineFunction(name, loaded) {
					t.Fatalf("could not redefine %q", name)
				}
				var ran bool
				var err error
				if inPlace {
					ran, err = rr.RunFunctionBodyInPlace(ctx, name)
				} else {
					args := append([]string(nil), rr.Params...)
					ran, err = rr.CallFunction(ctx, name, args...)
				}
				if err != nil || !ran {
					t.Fatalf("resolve: ran=%v err=%v", ran, err)
				}
				return rr.ExitStatus()
			})
		}
	}

	const src = "stub() { zzresolve; }\nstub a b\necho \"st=$?\"\necho \"leak=[${loc-unset}]\"\n"

	out, _ := run(t, src, install(false))
	if want := "depth=2\nargs=<a b>\nst=7\nleak=[unset]\n"; out != want {
		t.Errorf("called: got %q, want %q", out, want)
	}

	out, _ = run(t, src, install(true))
	// One frame rather than two, and everything else the same: the
	// positional parameters are the frame's already, the status is the
	// body's, and the local unwound with the call that was open all along.
	if want := "depth=1\nargs=<a b>\nst=7\nleak=[unset]\n"; out != want {
		t.Errorf("in place: got %q, want %q", out, want)
	}
}

// A name that is not a function is not run, and says so rather than failing.
//
// The same contract CallFunction keeps, and it matters for the same reason: a
// caller that has just resolved something is entitled to find out that it did
// not resolve to a function.
func TestABodyRunInPlaceReportsANameThatIsNotAFunction(t *testing.T) {
	var ran bool
	out, _ := run(t, "zzask\n", func(r *Runner) {
		r.Register("zzask", func(rr *Runner, ctx context.Context, _ []string) int {
			var err error
			ran, err = rr.RunFunctionBodyInPlace(ctx, "zznosuch")
			if err != nil {
				t.Fatal(err)
			}
			return 0
		})
	})
	if ran {
		t.Errorf("ran = true for a name that is not a function")
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("got %q, want nothing said", out)
	}
}
