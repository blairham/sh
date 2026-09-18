// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// OPTERR, which is this shell's alone: setting it to zero turns the `getopts`
// diagnostic off without moving to the silent form (#2945).
//
// interp proves what the axis does; this file pins that this preset answers
// it yes and that the answer reaches the builtin through the dialect's own
// wording. Measured 2026-09-17 on 5.3.20, on 3.2.57 and under the name `sh`,
// as a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin on
// /dev/null — one line on stderr at OPTERR=1 and none at OPTERR=0, for a bad
// option and for a missing argument alike.
func TestGetoptsOptErrSilencesTheComplaint(t *testing.T) {
	for _, tc := range []struct{ name, src, msg string }{
		{"a bad option", "set -- -z\ngetopts 'a' o\n", "illegal option -- z"},
		{"a missing argument", "set -- -a\ngetopts 'a:' o\n", "option requires an argument -- a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range []struct {
				assign string
				quiet  bool
			}{
				{"OPTERR=1\n", false},
				{"OPTERR=0\n", true},
				{"unset OPTERR\n", false},
				{"OPTERR=\n", false},
				{"OPTERR=x\n", true},
				{"", false},
			} {
				out, _, err := preset.Combined(t, dialecttest.Base{
					Name: "bash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
				}, v.assign+tc.src)
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				if got := !strings.Contains(out, tc.msg); got != v.quiet {
					t.Errorf("%q: got %q, want silenced = %v", v.assign, out, v.quiet)
				}
			}
		})
	}
	// And what is silenced is the sentence and nothing else: the name, the
	// status and OPTARG are what they were.
	out, _, err := preset.Combined(t, dialecttest.Base{
		Name: "bash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
	}, "OPTERR=0\nset -- -z rest\ngetopts 'a' o\necho \"st=$? [$o] [${OPTARG-unset}] ind=$OPTIND\"\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "st=0 [?] [unset] ind=2\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
