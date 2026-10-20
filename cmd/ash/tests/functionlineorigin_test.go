// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// A function keeps the line count of the text it was written in. BusyBox ash
// counts the `-c` string's lines from 0 and a file's from 1, and a body
// numbers its lines as its own text does wherever it is called from: one a
// file defined, called from the string, names the file's line, and one the
// string defined, called from inside a file, names the string's (#6696).
//
// The noun is the text the body was written in. The deciding pairs hold the
// call fixed and move the definition: `g` from `g.inc` and `h` from the
// string are both called with a file being read or not, and each keeps its
// own count. `$LINENO` moves with the location, so it is the counter.
//
// Measured 2026-10-15 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b
// (BusyBox v1.37.0); every expected line is that shell's. This shell took one
// off the first kind (`eval: line 0:`, `L=2`) and added one to the second.
func TestAFunctionKeepsTheLineCountOfTheTextItWasWrittenIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ENV", "")
	t.Chdir(home)
	files := map[string]string{
		"s1.inc": "fe(){ eval \"nosuch_fn\"; }\n",
		"s3.inc": ":\n:\nfe(){ eval \"nosuch_fn\"; }\n",
		"g.inc":  ":\ng(){\necho L=$LINENO\ncd /nonexist_7\neval \"cd /nonexist_e\"\n}\n",
		"b.inc":  ":\n:\nh\n",
		"k.inc":  ":\nk(){ m(){\necho M=$LINENO; cd /nonexist_m; }; }\n",
		"e.inc":  "e\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const cant = ": No such file or directory\n"
	for _, c := range []struct{ name, program, stdout, stderr string }{
		{
			"eval in a function a file defined", ". ./s1.inc\nfe", "",
			"ash: eval: line 1: nosuch_fn: not found\n",
		},
		{
			"the same on the file's third line", ". ./s3.inc\n:\nfe", "",
			"ash: eval: line 3: nosuch_fn: not found\n",
		},
		{
			"a builtin and $LINENO in such a function", ". ./g.inc; g", "L=3\n",
			"ash: cd: line 4: can't cd to /nonexist_7" + cant +
				"ash: cd: line 5: can't cd to /nonexist_e" + cant,
		},
		{
			"a function the string defined, called from a file", "h(){\necho L=$LINENO\ncd /nonexist_b\neval \"cd /nonexist_h\"\n}\n. ./b.inc", "L=1\n",
			"ash: cd: line 2: can't cd to /nonexist_b" + cant +
				"ash: cd: line 3: can't cd to /nonexist_h" + cant,
		},
		{
			"a function defined by a function a file defined", ". ./k.inc; k; m", "M=3\n",
			"ash: cd: line 3: can't cd to /nonexist_m" + cant,
		},
		{
			"a function eval defined in the string, called from a file", "eval \"e(){\ncd /nonexist_v\n}\"; . ./e.inc", "",
			"ash: cd: line 1: can't cd to /nonexist_v" + cant,
		},
		// The control: a function the string defined and called keeps the
		// string's count from 0, as it always did.
		{
			"the string's own function", "f(){ echo F=$LINENO; cd /nonexist_f; }\n:\nf", "F=0\n",
			"ash: cd: line 0: can't cd to /nonexist_f" + cant,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			driver.MainArgs(sh, []string{"ash", "-c", c.program})
			if o.String() != c.stdout || e.String() != c.stderr {
				t.Errorf("stdout\n%s\nstderr\n%s\nwant stdout\n%s\nwant stderr\n%s", o.String(), e.String(), c.stdout, c.stderr)
			}
		})
	}
}
