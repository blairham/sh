// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The module that reaches the filesystem without ever naming a command, from
// the outside, through the flags a person types.
//
// `zsh/files` is nine builtins that each take a path, and the tests beside
// this one cover them. `zsh/mapfile` is the same hole in a shape no command
// has: one *parameter*, where a read is an expansion and a write is an
// assignment, so a boundary watching commands sees nothing at all. #1808
// predicted this row by name — `make sandbox` graded `module/mapfile-read`
// and `module/mapfile-write` as `inert` with the note *"not implemented yet
// — will need a gate when it is"* — and this is the gate arriving in the
// same change as the feature, which is the whole point of keeping that
// ledger.
//
// Every case is decided by the filesystem or by what reached the script, and
// never by a diagnostic, for the reason sandboxfiles_test.go gives: a check
// that cannot tell "refused" from "did not work" would let the next escape in
// by the same route. Each denied case has an allowed sibling below, without
// which they would all pass for a module that does nothing.

// mf runs a script with the module loaded, from inside the workspace.
func mf(t *testing.T, ws, policy, script string) outcome {
	t.Helper()
	return sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/mapfile\n"+script+"\n")
}

// untouched asks the filesystem whether a denied file still holds what it
// held, which is the question a refused overwrite and a refused append both
// turn on. Not "does it exist": a write that truncated and then refused to
// fill the file would leave the name in place and the contents gone.
func untouched(at string) bool {
	data, err := os.ReadFile(at)
	return err == nil && string(data) == "PRECIOUS"
}

func TestMapfileIsInsideTheBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		make   func(t *testing.T, at string)
		script string // %s is the path outside the workspace
		held   func(at string) bool
		want   string
	}{{
		name:   "read",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `print -r -- "${mapfile[%s]}"`,
		held:   func(at string) bool { return exists(at) },
		want:   "the contents did not reach the script",
	}, {
		name: "read through the roster's own spelling",
		make: func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		// The subscript is the whole of the module's read route, so a second
		// spelling of it is worth a row: a gate placed on the parameter's
		// name rather than on the access would pass one and fail the other.
		script: `k=%s; print -r -- "${mapfile[$k]}"`,
		held:   func(at string) bool { return exists(at) },
		want:   "an indirectly-spelled key is the same read",
	}, {
		name:   "write",
		make:   func(*testing.T, string) {},
		script: `mapfile[%s]=x`,
		held:   func(at string) bool { return !exists(at) },
		want:   "no file was created outside",
	}, {
		name:   "write over something",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `mapfile[%s]=x`,
		held:   func(at string) bool { return untouched(at) },
		want:   "the file still holds what it held",
	}, {
		name:   "unset unlinks",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: `unset "mapfile[%s]"`,
		held:   func(at string) bool { return exists(at) },
		want:   "the file is still there",
	}, {
		name: "append reads before it writes",
		make: func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		// `+=` is a read and a write in one word, and the read half is the
		// one that can leak: a gate on the write alone would refuse to save
		// the result after the contents had already been joined onto the
		// script's own value.
		script: `v=seen; mapfile[%s]+=$v; print -r -- "${mapfile[%s]}"`,
		held:   func(at string) bool { return untouched(at) },
		want:   "the file still holds what it held",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			outside, ws, policy := filesFixture(t)
			at := filepath.Join(outside, "target")
			c.make(t, at)
			got := mf(t, ws, policy, strings.ReplaceAll(c.script, "%s", at))
			if !c.held(at) {
				t.Errorf("%s escaped the boundary: want %s\nerrs = %q",
					c.name, c.want, got.errs)
			}
			if strings.Contains(got.out, "PRECIOUS") {
				t.Errorf("out = %q: the contents of a denied file reached the script", got.out)
			}
		})
	}
}

// The roster is a directory listing spelled as a parameter flag, so it
// answers to the rule a listing answers to and not to the one a read does.
//
// It is the half of this module that names no path at all, which is what
// makes it the row a gate is most likely to be missing: there is no operand
// to attach a check to. A policy that allows the workspace and nothing else
// must not let `${(k)mapfile}` enumerate the directory above it.
func TestTheMapfileRosterIsADirectoryListing(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	writeFile(t, filepath.Join(outside, "SECRETNAME"), "x")

	// From the denied directory. `cd` itself may or may not be permitted
	// there, and the claim is the same either way: the name of a file in a
	// directory the policy does not cover must not come back.
	denied := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+outside+"\nzmodload zsh/mapfile\nprint -r -- \"${(k)mapfile}\"\n")
	if strings.Contains(denied.out, "SECRETNAME") {
		t.Errorf("out = %q: the roster enumerated a directory the policy does not cover", denied.out)
	}

	// And the sibling, without which the line above passes for a roster that
	// is empty everywhere.
	writeFile(t, filepath.Join(ws, "ALLOWEDNAME"), "x")
	allowed := mf(t, ws, policy, `print -r -- "${(k)mapfile}"`)
	if !strings.Contains(allowed.out, "ALLOWEDNAME") {
		t.Errorf("out = %q: the roster did not list the allowed workspace\nerrs = %q",
			allowed.out, allowed.errs)
	}
}

// A denied read tells a script nothing about whether the file is there.
//
// This is the indistinguishability the gate turns on, and it is worth being
// precise about which pair it holds over. A refused read is **not** silent —
// reading a file's contents is an open, so it is reported in the same words a
// refused redirection gets, `open: refused: <path>`, which is the loud half
// and is deliberate. What must not differ is the *denied* pair: a path the
// policy covers that holds something, and a path it covers that holds
// nothing. If those two answered differently, the refusal would be an oracle
// for exactly what the policy is hiding — a script could walk a denied tree
// and learn its shape without reading a byte of it.
//
// The paths are normalized out before the comparison, since each run names
// its own operand and the wording quotes it back. Without that, this passes
// for a shell whose two answers agree on nothing but the path.
func TestADeniedMapfileReadSaysNothingAboutWhetherTheFileIsThere(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "PRECIOUS")
	missing := filepath.Join(outside, "nosuch")

	refused := mf(t, ws, policy, `print -r -- "[${mapfile[`+secret+`]}]" ${+mapfile[`+secret+`]}`)
	absent := mf(t, ws, policy, `print -r -- "[${mapfile[`+missing+`]}]" ${+mapfile[`+missing+`]}`)

	norm := func(s, path string) string { return strings.ReplaceAll(s, path, "PATH") }
	if refused.out != absent.out {
		t.Errorf("refused = %q, absent = %q: the value a denied read produces differs",
			refused.out, absent.out)
	}
	if got, want := norm(refused.errs, secret), norm(absent.errs, missing); got != want {
		t.Errorf("refused = %q, absent = %q: a script can tell a denied file that exists "+
			"from a denied one that does not", got, want)
	}
	// And the wording is the open's, not a probe's silence.
	if !strings.Contains(refused.errs, "refused") {
		t.Errorf("errs = %q, want the refusal reported the way a redirection's read is", refused.errs)
	}
}

// A refused *write* is not quiet, and that is deliberate: there is no honest
// way to carry on, since a file that was not written is not there, and
// AllowModify has already reported it in the words a refused redirection
// gets. Saying nothing would leave a script believing it had saved something.
func TestARefusedMapfileWriteSaysSo(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	at := filepath.Join(outside, "made")

	got := mf(t, ws, policy, `mapfile[`+at+`]=x`)
	if !strings.Contains(got.errs, "refused") {
		t.Errorf("errs = %q, want the refusal reported the way a redirection's is", got.errs)
	}
	if exists(at) {
		t.Error("the file was created")
	}
}

// The other half of every case above, without which they would all pass for a
// module that simply does not work.
//
// One script rather than a table, for the reason the files module's sibling
// gives: the operations compose — a file is written, read back, appended to
// and removed — so a single run exercises each against the state the last one
// left, which is how a caller uses them.
func TestMapfileStillWorksWhereThePolicyAllowsIt(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)

	got := mf(t, ws, policy, strings.Join([]string{
		`mapfile[made]=abc`,
		`print -r -- "wrote=[${mapfile[made]}]"`,
		`mapfile[made]+=def`,
		`print -r -- "appended=[${mapfile[made]}]"`,
		`unset "mapfile[made]"`,
		`print -r -- "gone=${+mapfile[made]}"`,
	}, "\n"))

	for _, want := range []string{"wrote=[abc]", "appended=[abcdef]", "gone=0"} {
		if !strings.Contains(got.out, want) {
			t.Errorf("out = %q, want it to contain %q\nerrs = %q", got.out, want, got.errs)
		}
	}
}
