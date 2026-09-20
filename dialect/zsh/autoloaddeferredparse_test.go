// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A `$( … )` body that will not parse costs the autoloaded function and not the
// whole outer script (#3895).
//
// zsh reads a function file **whole** when it loads it, so a refused
// substitution in one is caught by the load: the definition never happens and
// the script carries on with the name still waiting to be loaded. This engine
// reads a substitution body at expansion time, so the same failure arrived from
// inside a body already running, escaped the call, and ended the script.
//
// Measured 2026-09-20, `env -i -u FPATH PATH=/usr/bin:/bin LC_ALL=C zsh t.zsh`
// with the file on `$fpath` and the script `echo BEFORE; f; echo "AFTER $?"`:
//
//	file                zsh 5.9.2                          here, before
//	v=$(echo hi; for)   BEFORE, refusals, `AFTER 1`, 0     BEFORE only, exit 1
//	echo X${NOPE?gone}  BEFORE, the refusal, exit 1        the same
//	echo X${NOPE} (-u)  BEFORE, the refusal, exit 1        the same
//	exit 4              BEFORE, exit 4                     the same
//	return 7            BEFORE, `AFTER 7`, 0               the same
//
// The middle three are the controls and they are why the boundary catches one
// kind of failure rather than every fatal one: a **run-time** error inside an
// autoloaded body ends the script in zsh exactly as it does anywhere else, and
// a request to stop is still a request to stop. See
// interp.Runner.GiveUpTheDeferredParse.
func TestARefusedSubstitutionInAnAutoloadFileCostsTheFunction(t *testing.T) {
	for _, c := range []struct {
		name, file, src, want string
		status                int
	}{
		{
			name: "the script carries on",
			file: `v=$(echo hi; for)`,
			src:  `echo BEFORE; g; echo "AFTER $?"`,
			want: "AFTER 1",
		},
		{
			// zsh leaves the name unloaded, so the next call reads the file
			// and reports again. Without the declaration going back, the
			// second call ran the body this engine had already defined and
			// escaped the boundary — the script died on the call *after* the
			// one that was contained.
			name: "and so does the call after it",
			file: `v=$(echo hi; for)`,
			src:  `g; echo "M $?"; g; echo "AFTER $?"`,
			want: "AFTER 1",
		},
		{
			// An ordinary failing status rather than a stop, which is what
			// `||` seeing it proves.
			name: "the status is one a script can catch",
			file: `v=$(echo hi; for)`,
			src:  `g || echo caught; echo "AFTER $?"`,
			want: "caught",
		},
		{
			// A subshell around the call is contained too: the failure records
			// its stop in the shared box rather than on this runner, and the
			// boundary drains it.
			name: "inside a subshell",
			file: `v=$(echo hi; for)`,
			src:  `( g ); echo "AFTER $?"`,
			want: "AFTER 1",
		},
		{
			name: "and inside a substitution",
			file: `v=$(echo hi; for)`,
			src:  `w=$(g); echo "AFTER $?"`,
			want: "AFTER 1",
		},
		// The control that must not be caught, and the one that must not
		// change: a request to stop still stops, and an ordinary `return` is
		// an ordinary return.
		{
			name:   "a request to stop is not caught",
			file:   `exit 4`,
			src:    `echo BEFORE; g; echo "AFTER $?"`,
			want:   "BEFORE",
			status: 4,
		},
		{
			name: "and a return is a return",
			file: `return 7`,
			src:  `g; echo "AFTER $?"`,
			want: "AFTER 7",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := autoloadFileDir(t, "g", c.file)
			out, st := runZsh(t, dir, "fpath=("+dir+"/fns); autoload -Uz g; "+c.src)
			if !strings.Contains(out, c.want) || st != c.status {
				t.Errorf("%q said %q (status %d), want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// A run-time error in an autoloaded body still ends the script, which is the
// half the boundary must not take with it.
//
// Same measurement run: zsh prints `BEFORE`, the diagnostic, and exits 1 for
// both of these — no `AFTER` — so a boundary catching every fatal error here
// would have been wrong in the direction nothing else would notice.
func TestARunTimeErrorInAnAutoloadFileStillEndsTheScript(t *testing.T) {
	for _, c := range []struct{ name, file, src string }{
		{"a diagnostic operand", `echo X${NOPE?gone}`, `echo BEFORE; g; echo "AFTER $?"`},
		{"an unset parameter under -u", `echo X${NOPE}`, `set -u; echo BEFORE; g; echo "AFTER $?"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := autoloadFileDir(t, "g", c.file)
			out, st := runZsh(t, dir, "fpath=("+dir+"/fns); autoload -Uz g; "+c.src)
			if strings.Contains(out, "AFTER") || st == 0 {
				t.Errorf("%q said %q (status %d), want the script ended before AFTER", c.src, out, st)
			}
		})
	}
}

// autoloadFileDir writes one function file into a scratch `fns` directory and
// reports the directory holding it.
func autoloadFileDir(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	fns := filepath.Join(dir, "fns")
	if err := os.MkdirAll(fns, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fns, name), []byte(body+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}
