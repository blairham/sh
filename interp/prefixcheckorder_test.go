// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// prefixOrderRun runs src with the three refusal axes held at answers that
// cost the command nothing, so that what the rows below measure is the
// *order* the frozen name is checked in and nothing else.
func prefixOrderRun(t *testing.T, src string, first Answer) (stdout, stderr string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.PrefixToARegularBuiltinIsRefused = Yes
	sem.PrefixRefusalFatality = PrefixRefusalNeverFatal
	sem.PrefixRefusalCostsTheCommand = No
	sem.PrefixToAFrozenNameIsCheckedFirst = first
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Diagnostics: &Diagnostics{
			Location:         LocationLineWord,
			ReadonlyVariable: "%s: readonly variable",
			NotFound:         "%[1]s: command not found",
		},
		Dir: t.TempDir(), Name: "testsh",
		Vars: map[string]string{"PATH": "/bin:/usr/bin"},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// When a frozen name in an assignment prefix is checked: before the command's
// values are expanded and its redirections opened, or after.
//
// Two probes agreeing on one boundary is what makes it a boundary rather than
// a quirk of arithmetic — a redirection that cannot be opened reaches the
// same order with no expression in it at all (#1943).
func TestWhenAFrozenPrefixIsChecked(t *testing.T) {
	for _, tc := range []struct {
		name, src            string
		firstOut, firstErr   string
		expandOut, expandErr string
	}{
		{
			// The value never expands under the first reading, so the
			// division is silent there and is the only complaint under the
			// other. That is the row that says this is not merely a
			// reordering of two diagnostics.
			"a value that will not expand",
			"readonly x=1\nx=$((1/0)) /bin/echo RAN\n",
			"RAN\n", "testsh: line 2: x: readonly variable\n",
			"RAN\n", "testsh: line 2: division by zero\n",
		},
		{
			// The same in front of a function, where the prefix is otherwise
			// discarded — which is what once made the two dispatch routes
			// answer the order differently (#1940).
			"a value that will not expand in front of a function",
			"f() { echo IN-F; }\nreadonly x=1\nx=$((1/0)) f\n",
			"IN-F\n", "testsh: line 3: x: readonly variable\n",
			"IN-F\n", "testsh: line 3: division by zero\n",
		},
		{
			// No expression at all, so the boundary is the redirection's.
			// Under the first reading the name is named and *then* the file;
			// under the other only the file.
			"a redirection that cannot be opened",
			"readonly x=1\nx=2 /bin/echo RAN >/nope/f\n",
			"", "testsh: line 2: x: readonly variable\n" +
				"testsh: line 2: cannot create /nope/f: No such file or directory\n",
			"", "testsh: line 2: cannot create /nope/f: No such file or directory\n",
		},
		{
			// The control both readings answer alike: a prefix whose value
			// expands, in front of a command that runs. The refusal is the
			// same one report either way, which is what says the axis is
			// about the order and not about the refusal.
			"a value that expands",
			"readonly x=1\nx=2 /bin/echo RAN\n",
			"RAN\n", "testsh: line 2: x: readonly variable\n",
			"RAN\n", "testsh: line 2: x: readonly variable\n",
		},
		{
			// And a prefix with nothing frozen in it: the axis is never
			// reached, so a value that will not expand reports its own
			// failure under both answers.
			"nothing frozen in the prefix",
			"y=$((1/0)) /bin/echo RAN\n",
			"RAN\n", "testsh: line 1: division by zero\n",
			"RAN\n", "testsh: line 1: division by zero\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := prefixOrderRun(t, tc.src, Yes)
			if out != tc.firstOut || errs != tc.firstErr {
				t.Errorf("checked first: stdout %q stderr %q, want %q and %q",
					out, errs, tc.firstOut, tc.firstErr)
			}
			out, errs, _ = prefixOrderRun(t, tc.src, No)
			if out != tc.expandOut || errs != tc.expandErr {
				t.Errorf("expanded first: stdout %q stderr %q, want %q and %q",
					out, errs, tc.expandOut, tc.expandErr)
			}
		})
	}
}

// The axis is read once a name in the prefix is frozen, and not before.
//
// The rows that must stay quiet are the point: an ordinary command, a command
// with a prefix that names nothing frozen, and an assignment with no command
// after it are the shapes a script is actually made of.
func TestThePrefixOrderAxisIsAskedOnlyOverAFrozenName(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refused   bool
	}{
		{"a frozen name in the prefix", "readonly x=1\nx=2 /bin/echo RAN\n", true},
		{"an unfrozen name in the prefix", "y=2 /bin/echo RAN\n", false},
		{"a frozen name and no prefix", "readonly x=1\n/bin/echo RAN\n", false},
		{"a frozen name assigned on its own line", "readonly x=1\nx=2\n", false},
		{"no prefix and no frozen name", "/bin/echo RAN\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := syntax.Parse(tc.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			sem := permissive()
			sem.PrefixToARegularBuiltinIsRefused = Yes
			sem.PrefixRefusalFatality = PrefixRefusalNeverFatal
			sem.PrefixRefusalCostsTheCommand = No
			sem.PrefixToAFrozenNameIsCheckedFirst = Unspecified
			var out, errs bytes.Buffer
			r := newTestRunner(t, &Runner{
				Stdout: &out, Stderr: &errs, Semantics: &sem,
				Diagnostics: &Diagnostics{
					Location:         LocationLineWord,
					ReadonlyVariable: "%s: readonly variable",
					NotFound:         "%[1]s: command not found",
				},
				Dir: t.TempDir(), Name: "testsh",
				Vars: map[string]string{"PATH": "/bin:/usr/bin"},
			})
			if _, rerr := r.Run(context.Background(), f); rerr != nil {
				t.Fatalf("run: %v", rerr)
			}
			said := strings.Contains(errs.String(), "checked before the command")
			if said != tc.refused {
				t.Fatalf("stderr %q, want the axis %s", errs.String(),
					map[bool]string{true: "reported", false: "not reported"}[tc.refused])
			}
		})
	}
}
