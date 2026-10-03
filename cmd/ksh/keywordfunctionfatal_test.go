// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// runKshScript runs body as the script file s.sh in a directory of its own,
// with p.sh beside it for the rows that source one, and answers the two
// streams with the directory taken out.
func runKshScript(t *testing.T, body string) (out, errs string, code int) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"s.sh": body, "p.sh": "echo p1\n: ${NOPE?pfail}\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	var o, e strings.Builder
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	code = driver.MainArgs(sh, []string{"ksh", "s.sh"})
	return o.String(), e.String(), code
}

// A fatal error inside a `function name { … }` body ends the call and not the
// script, and the diagnostic names the keyword calls it was raised under.
// Measured 2026-10-02 on ksh93u+ 2012-08-01 over a script file under `env -i
// PATH=/usr/bin:/bin` (#5508). See
// interp.Semantics.FatalErrorEndsAtAKeywordFunctionCall and
// interp.Diagnostics.KeywordCallIsACallStackComponent.
func TestAFatalErrorEndsAtAKeywordFunctionCall(t *testing.T) {
	for _, c := range []struct{ name, src, out, errs string }{
		{
			"a readonly assignment",
			"readonly y; function f { y=1; echo in; }; f; echo st=$?\n",
			"st=1\n", "s.sh[1]: f: line 1: y: is read only\n",
		},
		{
			"a parameter error",
			"function f { : ${u?gone}; echo in; }; f; echo st=$?\n",
			"st=1\n", "s.sh[1]: f: line 1: u: gone\n",
		},
		{
			"a usage error keeps its status",
			"function f { unset -Z; echo in; }; f; echo st=$?\n",
			"st=2\n", "s.sh[1]: f[1]: unset: -Z: unknown option\nUsage: unset [-nfv] name...\n",
		},
		{
			"a POSIX function inside unwinds to the keyword call",
			"readonly y; function f { g; echo f-after; }; g() { y=1; echo g-in; }; f; echo st=$?\n",
			"st=1\n", "s.sh[1]: f: line 1: y: is read only\n",
		},
		{
			"nested keyword calls stop at the inner one",
			"readonly y; function f { y=1; }\nfunction g {\n  f\n  echo g $?\n}\ng; echo st=$?\n",
			"g 1\nst=0\n", "s.sh[6]: g[3]: f: line 1: y: is read only\n",
		},
		{
			"a sourced file inside a keyword call",
			"function f {\n  . ./p.sh\n}\nf\necho st=$?\n",
			"p1\nst=1\n", "s.sh[4]: f[2]: .: line 2: NOPE: pfail\n",
		},
		{
			"a keyword call from an eval",
			"function f { y=1; }\nreadonly y\neval \"f; echo e \\$?\"\n",
			"e 1\n", "s.sh[3]: eval[1]: f: line 1: y: is read only\n",
		},
		{
			"an eval inside a keyword call",
			"readonly y\nfunction f {\n  eval \"echo a\n  y=1\"\n  echo after\n}\nf\n",
			"a\nafter\n", "s.sh[7]: f[3]: eval: line 2: y: is read only\n",
		},
		{
			"a builtin's location",
			"function k {\n  cd /nonexistent/x\n}\nk\n",
			"", "s.sh[4]: k[2]: cd: /nonexistent/x: [No such file or directory]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := runKshScript(t, c.src)
			if out != c.out || errs != c.errs {
				t.Errorf("%s\n got %q, %q\nwant %q, %q", c.src, out, errs, c.out, c.errs)
			}
		})
	}
}

// The controls: a POSIX-form body is no boundary and names no frame, and a
// request to stop is not caught.
func TestAPosixFunctionAndARequestToStopStillEndTheScript(t *testing.T) {
	for _, c := range []struct {
		name, src, out, errs string
		code                 int
	}{
		{"the POSIX form", "readonly y; f() { y=1; echo in; }; f; echo st=$?\n", "", "s.sh: line 1: y: is read only\n", 1},
		{"exit", "function f { exit 3; }; f; echo st=$?\n", "", "", 3},
		{"a POSIX function sourcing a file", "g() { . ./p.sh; }\ng\necho st=$?\n", "p1\nst=1\n", "s.sh[1]: .: line 2: NOPE: pfail\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runKshScript(t, c.src)
			if out != c.out || errs != c.errs || code != c.code {
				t.Errorf("%s\n got %q, %q, %d\nwant %q, %q, %d", c.src, out, errs, code, c.out, c.errs, c.code)
			}
		})
	}
}
