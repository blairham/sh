// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// One dialect names the function a message came from where the file's name
// would go, and counts the line within the function rather than the file.
//
// The count is the offset from the line the function was written on, so a
// body on the same line as its `f() {` is offset zero — and the number is
// left out entirely there rather than written as a nought.
func TestAMessageMayBeNamedForItsFunction(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"the second line of the body",
			"# c\nf() {\n  true\n  nosuchcmd\n}\ntrue\nf\n", "f:2: ",
		},
		{
			"the first line of the body",
			"f() {\n  nosuchcmd\n}\nf\n", "f:1: ",
		},
		{
			// Offset zero, so no number at all.
			"a body on the line of its own definition",
			"f() { nosuchcmd; }\nf\n", "f: ",
		},
		{
			// The innermost function, not the one that called it.
			"the function it happened in, not the caller",
			"f() {\n  nosuchcmd\n}\ng() {\n  f\n}\ntrue\ng\n", "f:1: ",
		},
		{
			// Outside any function the file is named and the line is the
			// file's, which is what every dialect does.
			"outside a function, the file and its own line",
			"f() {\n  true\n}\nf\nnosuchcmd\n", "sh:5: ",
		},
		{
			// And after the function returns, the count goes back to the
			// file — the offset must be given up along with the name.
			"after the function returns",
			"f() {\n  true\n}\nf\ntrue\nnosuchcmd\n", "sh:6: ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := funcLocRun(t, c.src, true); !strings.HasPrefix(got, c.want) {
				t.Errorf("said %q, want it to start %q", got, c.want)
			}
		})
	}
}

// Every other dialect names the file and counts from the top of it, inside a
// function as much as outside.
func TestWithoutThatAnswerTheFileIsNamed(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"in a function", "f() {\n  true\n  nosuchcmd\n}\nf\n", "sh:3: "},
		{"on one line", "f() { nosuchcmd; }\nf\n", "sh:1: "},
		{"outside one", "nosuchcmd\n", "sh:1: "},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := funcLocRun(t, c.src, false); !strings.HasPrefix(got, c.want) {
				t.Errorf("said %q, want it to start %q", got, c.want)
			}
		})
	}
}

func funcLocRun(t *testing.T, src string, names bool) string {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	dg := Diagnostics{Location: LocationTightLine, LocationNamesTheFunction: names}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
		Stdout: &strings.Builder{}, Stderr: &buf,
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
