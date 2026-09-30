// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `emulate … -c` runs borrowed text, and whether the string's trace keeps out
// of the redirection on the command is decided by tracing **as it stood when
// the command was reached**, before the emulation and its own option words
// move it (#5260). See interp.Runner.HoldTracingForBorrowedText.
//
// Each row redirects the command's standard error to a file and then prints
// the file, so a trace that went to standard error comes before `file[`.
// Measured on zsh 5.9.2, `-f`, 2026-09-30; on main before this change the
// three rows that change tracing inside the command read the other way.
func TestEmulateTracesByTheStateItWasReachedIn(t *testing.T) {
	rows := []struct{ name, src, want string }{
		{
			"-o xtrace: the trace follows the redirection", "emulate zsh -o xtrace -c ':' 2>>f",
			"file[+> :]\n",
		},
		{"-x likewise", "emulate zsh -x -c ':' 2>>f", "file[+> :]\n"},
		{
			"and a function inside it", "emulate zsh -o xtrace -c 'g(){ :; }; g' 2>>f",
			"file[+> g\n+> :]\n",
		},
		// Text nested inside answers from its own entry: the inner eval was
		// reached tracing, so its trace keeps out of its own redirection and
		// lands in the command's.
		{
			"an eval inside answers for itself", "emulate zsh -o xtrace -c 'eval \":\" 2>>g' 2>>f",
			"file[+> eval :\n+> :]\n",
		},
		{
			"+o xtrace from a tracing shell keeps out of it",
			"setopt xtrace; emulate zsh +o xtrace -c 'setopt xtrace; :' 2>>f; setopt noxtrace 2>/dev/null",
			"+> emulate zsh +o xtrace -c 'setopt xtrace; :'\n+> :\n+> setopt noxtrace\nfile[]\n",
		},
		{
			"control: tracing already on keeps out of it",
			"setopt xtrace; emulate zsh -c ':' 2>>f; setopt noxtrace 2>/dev/null",
			"+> emulate zsh -c :\n+> :\n+> setopt noxtrace\nfile[]\n",
		},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			src := "PS4='+> '\n" + row.src + "\nprint -r -- \"file[$(<f)]\"\n"
			out, st := runZsh(t, t.TempDir(), src)
			if st != 0 || out != row.want {
				t.Errorf("out %q status %d, want %q", out, st, row.want)
			}
		})
	}
}
