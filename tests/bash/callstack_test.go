// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The three arrays bash keeps in step, and the two rules that are easy to get
// almost right: the bottom frame is called `main`, and it is only there when
// the shell was given a file to run.
func TestBashNamesTheCallStack(t *testing.T) {
	for _, tc := range []struct {
		name, src, file, want string
	}{
		{
			"inside a function, from a script",
			`f(){ echo "[${FUNCNAME[*]}] [${BASH_SOURCE[*]}]"; }; f`,
			"/s/main.sh", "[f main] [/s/main.sh /s/main.sh]",
		},
		{
			// No file, so no bottom frame — and so no `main`. Reported by
			// bash as the function alone, which is the case a stack that
			// always appends `main` gets wrong.
			"inside a function, from -c",
			`f(){ echo "[${FUNCNAME[*]}]"; }; f`,
			"", "[f]",
		},
		{
			// Absent outside a function rather than empty, which is a
			// different thing to a script testing it.
			"outside a function",
			`echo "n=${#FUNCNAME[@]}"`,
			"/s/main.sh", "n=0",
		},
		{
			"nested, innermost first",
			`g(){ echo "${FUNCNAME[*]}"; }; f(){ g; }; f`,
			"/s/main.sh", "g f main",
		},
		{
			// Nothing called the script, so its own frame reports line 0.
			"the lines each frame was called from",
			`f(){ echo "${BASH_LINENO[*]}"; }; f`,
			"/s/main.sh", "1 0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runWithScriptFile(t, tc.src, tc.file); got != tc.want {
				t.Errorf("out = %q, want %q", got, tc.want)
			}
		})
	}
}

func runWithScriptFile(t *testing.T, src, file string) string {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var out bytes.Buffer
	sem, dg := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{Semantics: &sem, Diagnostics: &dg, Stdout: &out, Stderr: &out, Name: "testsh", Dialect: presetDialect()}
	bash.Apply(r)
	if file != "" {
		r.SetScriptFile(file)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	return strings.TrimSpace(out.String())
}
