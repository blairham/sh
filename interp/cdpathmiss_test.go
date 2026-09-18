// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A CDPATH search that misses: the operand as it stands, or a failure (#2896).
//
// The axis is moved both ways over the same rows, because a fallback that is
// always taken cannot be told from one nobody consults. The rows that reach
// the destination under both answers are the controls — they say the axis is
// asked where the search decides something and nowhere else.
func TestACdpathSearchThatMisses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		replaces Answer
		cdpath   string
		operand  string
		arrives  bool
	}{
		{
			name: "a miss falls back", replaces: No,
			cdpath: "./pool", operand: "here", arrives: true,
		},
		{
			name: "a miss is a failure", replaces: Yes,
			cdpath: "./pool", operand: "here", arrives: false,
		},
		{
			// A trailing slash changes nothing: it is the same operand and
			// the same search.
			name: "a miss with a slash falls back", replaces: No,
			cdpath: "./pool", operand: "here/", arrives: true,
		},
		{
			name: "a miss with a slash is a failure", replaces: Yes,
			cdpath: "./pool", operand: "here/", arrives: false,
		},
		{
			// A bare dot is not a `./` prefix, so it is searched like any
			// other relative operand: with an entry holding no `.` to find,
			// the shell with no fallback refuses it. Measured rather than
			// incidental — ksh93u+ answers `cd: .: [No such file or
			// directory]` for exactly this.
			name: "an unfindable operand falls back", replaces: No,
			cdpath: "./nosuch", operand: "here", arrives: true,
		},
		{
			name: "an unfindable operand is a failure", replaces: Yes,
			cdpath: "./nosuch", operand: "here", arrives: false,
		},
		// The controls.
		{
			name: "a hit arrives, fallback", replaces: No,
			cdpath: "./pool", operand: "sub", arrives: true,
		},
		{
			name: "a hit arrives, no fallback", replaces: Yes,
			cdpath: "./pool", operand: "sub", arrives: true,
		},
		{
			name: "a dot-led operand is not searched, fallback", replaces: No,
			cdpath: "./pool", operand: "./here", arrives: true,
		},
		{
			name: "a dot-led operand is not searched, no fallback", replaces: Yes,
			cdpath: "./pool", operand: "./here", arrives: true,
		},
		{
			name: "an empty CDPATH is no search, fallback", replaces: No,
			cdpath: "", operand: "here", arrives: true,
		},
		{
			name: "an empty CDPATH is no search, no fallback", replaces: Yes,
			cdpath: "", operand: "here", arrives: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, d := range []string{
				filepath.Join(dir, "pool", "sub"),
				filepath.Join(dir, "here"),
			} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			sem := PosixSemantics()
			sem.CdpathAnnouncesTheDirectory = No
			sem.CdpathReplacesTheRelativeLookup = tc.replaces
			errOut := &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: &strings.Builder{}, Stderr: errOut,
				Vars: map[string]string{"CDPATH": tc.cdpath},
			})
			f, err := syntax.Parse("cd "+tc.operand+"\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if arrived := r.Dir != dir; arrived != tc.arrives {
				t.Errorf("cd %s with CDPATH=%q left Dir %q, arrived=%v want %v",
					tc.operand, tc.cdpath, r.Dir, arrived, tc.arrives)
			}
			// A refusal says the ordinary missing-directory sentence, so a
			// script trapping on `cd` cannot tell it from a path that is
			// really not there.
			said := errOut.String()
			if !tc.arrives && !strings.Contains(said, "cd: "+tc.operand+": ") {
				t.Errorf("stderr %q, want the operand named in the ordinary refusal", said)
			}
			if tc.arrives && said != "" {
				t.Errorf("stderr %q, want nothing said", said)
			}
		})
	}
}
