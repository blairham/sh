// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// **Every body a real shell would fork is a process of its own, and nothing
// else is.** A function call and the shell itself share one; a subshell, a
// command substitution, a non-last pipeline element and a background job each
// get a new one, and what a body registers to happen at its exit happens when
// the body ends — before the line after it runs.
func TestAForkedBodyIsAProcessOfItsOwn(t *testing.T) {
	var out bytes.Buffer
	sem := interp.PosixSemantics()
	dial := syntax.Dialect{}
	r := newTestRunner(t, &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &sem, Dialect: &dial, Env: testPATH()})
	var mu sync.Mutex
	seen := map[*interp.Process]int{}
	name := func(p *interp.Process) string {
		if p == nil {
			return "shell"
		}
		mu.Lock()
		defer mu.Unlock()
		n, ok := seen[p]
		if !ok {
			n = len(seen) + 1
			seen[p] = n
		}
		return fmt.Sprint("p", n)
	}
	r.Register("whoami", func(r *interp.Runner, _ context.Context, args []string) int {
		_, _ = fmt.Fprintf(r.Stdout, "%s %s\n", args[0], name(r.Process()))
		return 0
	})
	r.Register("atexit", func(r *interp.Runner, _ context.Context, args []string) int {
		w, said := r.Stdout, args[0]
		r.Process().AtExit(func() { _, _ = fmt.Fprintf(w, "exit %s\n", said) })
		return 0
	})
	src := `whoami top
f() { whoami func; }
f
( whoami sub; f )
x=$(whoami subst); echo "$x"
whoami left | cat
{ whoami job; } &
wait
( atexit a; echo in )
echo after
atexit shell
echo end`
	file, err := syntax.Parse(src, dial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	want := `top shell
func shell
sub p1
func p1
subst p2
left p3
job p4
in
exit a
after
end
`
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(out.String(), "exit shell") {
		t.Error("the shell's own process ran an exit it does not have")
	}
}
