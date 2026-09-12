// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What a prompt rendering leaves the terminal set to is the *shell's* state,
// so the next rendering's `%b` writes back a color an earlier one chose.
//
// The rule and its flags are in interp; these are the bytes, measured on zsh
// 5.9.2 under `TERM=xterm-256color` on 2026-09-12. The first row is the whole
// of the claim — the `%b` is alone in its own rendering and still writes the
// color back:
//
//	v='%F{070}'; w='%b'
//	print -rn -- "${(%%)v}"; print -rn -- "${(%%)w}"
//	\e[38;5;70m  \e[0m\e[38;5;70m
//
// This is #2113. powerlevel10k binary-searches its own prompt width through
// `${(%%)…}` many times before the prompt is drawn, so by the time it is the
// state is never empty; a walker that started each rendering empty wrote
// `\e[0m\e[49m\e[38;5;NNNm` where zsh writes four escapes, and every segment
// boundary in the drawn prompt came out a color short.
func TestAPromptRenderingRestoresWhatAnEarlierOneSet(t *testing.T) {
	dir := t.TempDir()
	const decl = `v='%F{070}'; w='%b'; k='%K{021}'; c='%f'; `
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			name: "two renderings",
			src:  `print -rn -- "${(%%)v}"; print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[0m\x1b[38;5;70m",
		},
		{
			// The same text in one rendering, so the pair says the boundary
			// is what changed and not the restore itself.
			name: "one rendering",
			src:  `t=$v$w; print -rn -- "${(%%)t}"`,
			want: "\x1b[38;5;70m\x1b[0m\x1b[38;5;70m",
		},
		{
			// A clear reaches across the boundary too: `%f` in a rendering of
			// its own leaves the foreground with nothing to write back.
			name: "a clear in between",
			src:  `print -rn -- "${(%%)v}"; print -rn -- "${(%%)c}"; print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[39m\x1b[0m",
		},
		{
			// Two layers set in one rendering and restored in the next, which
			// is what says the state is the whole of it and not the last
			// sequence written.
			name: "both layers",
			src:  `t=$v$k; print -rn -- "${(%%)t}"; print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[48;5;21m\x1b[0m\x1b[38;5;70m\x1b[48;5;21m",
		},
		{
			// The single `%` flag draws the same escapes, so it keeps the
			// same state: a shell where only `(%%)` carried would answer
			// this one three escapes.
			name: "the single flag carries too",
			src:  `print -rn -- "${(%)v}"; print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[0m\x1b[38;5;70m",
		},
		{
			name: "print -P carries too",
			src:  `print -Pn -- "$v"; print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[0m\x1b[38;5;70m",
		},
		{
			// Down into a subshell, which is the direction a copy gives for
			// free.
			name: "a subshell sees the shell's state",
			src:  `print -rn -- "${(%%)v}"; (print -rn -- "${(%%)w}")`,
			want: "\x1b[38;5;70m\x1b[0m\x1b[38;5;70m",
		},
		{
			// And not back out of one, which is the direction a shared
			// pointer would have got wrong.
			name: "a subshell's rendering does not reach the parent",
			src:  `(print -rn -- "${(%%)v}"); print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[0m",
		},
		{
			name: "nor does a command substitution's",
			src:  `x=$(print -rn -- "${(%%)v}"); print -rn -- "$x"; print -rn -- "${(%%)w}"`,
			want: "\x1b[38;5;70m\x1b[0m",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, decl+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
