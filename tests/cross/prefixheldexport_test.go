// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A prefix entry held for the entries behind it carries the export attribute
// the command's route gives it, so a command substitution behind it hands a
// child what the route will. Measured 2026-10-30 from `-c` under `env -i
// PATH=/usr/bin:/bin` against ksh93u+ 2012-08-01, zsh 5.9.2, GNU bash 5.3.20,
// dash 0.5.12 and BusyBox ash 1.37.0 in the pinned image. This shell held each
// entry with the attribute the name already had: the first row was empty in
// every dialect, bash's function and `eval` rows were empty, and ksh93's read
// the old value through a name exported before the command (#7003). See
// interp/prefixheldexport.go.
func TestAHeldPrefixEntryCarriesTheExportItsRouteGivesIt(t *testing.T) {
	const fn = `f() { echo "w=$w"; }; `
	for _, c := range []struct {
		name, src string
		want      map[string]string
	}{
		{
			"a program's prefix is its environment",
			`v=old; v=new w=$(/usr/bin/printenv v) /usr/bin/printenv w`,
			map[string]string{"ksh": "new\n", "zsh": "new\n", "bash": "new\n", "dash": "new\n", "ash": "new\n"},
		},
		{
			"a function's",
			fn + `export v=old; v=new w=$(/usr/bin/printenv v) f`,
			map[string]string{"ksh": "w=\n", "zsh": "w=new\n", "bash": "w=new\n", "dash": "w=new\n", "ash": "w=new\n"},
		},
		{
			"a function's over a name nobody exported",
			fn + `v=old; v=new w=$(/usr/bin/printenv v) f`,
			map[string]string{"ksh": "w=\n", "zsh": "w=new\n", "bash": "w=new\n", "dash": "w=new\n", "ash": "w=new\n"},
		},
		{
			"a builtin's",
			`v=old; v=new w=$(/usr/bin/printenv v) eval 'echo "w=$w"'`,
			map[string]string{"ksh": "w=\n", "zsh": "w=\n", "bash": "w=new\n", "dash": "w=\n", "ash": "w=\n"},
		},
		{
			"a builtin's over a name exported before it",
			`export v=old; v=new w=$(/usr/bin/printenv v) eval 'echo "w=$w"'`,
			map[string]string{"ksh": "w=\n", "zsh": "w=new\n", "bash": "w=new\n", "dash": "w=new\n", "ash": "w=new\n"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for name, want := range c.want {
				out, st, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
				if err != nil {
					t.Fatal(err)
				}
				if st != 0 || out != want {
					t.Errorf("%s: got %q (status %d), want %q", name, out, st, want)
				}
			}
		})
	}
}
