// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An `_arguments` option spec whose exclusion list names its own option — the
// `(-f --force){-f,--force}'[force overwrite]'` idiom the shipped completions
// write everywhere — and what a list does while the option is still under the
// cursor (#3230).
//
// Every row was measured on zsh 5.9.2, 2026-09-18 through a pseudo-terminal
// from inside a real `zle -C` widget, asking `comparguments -O` and reading
// the `next` array back. Twenty shapes, one list at a time, and the rule they
// agree on is in compargumentsline.go's spend: **an option still under the
// cursor shuts off only the single-letter names**, and it never shuts off
// itself.
//
// It is measured and not explained, which the file comment says too. What
// makes it worth writing in rather than leaving alone is that it is the
// commonest idiom in the shipped completions, and applying the list as
// written took `--force` away from `git checkout -f<TAB>` where zsh offers it.

// offeredAt reports the `next` array `comparguments -O` answers with, so a
// row that lost the wrong option names it.
func offeredAt(t *testing.T, specs, line string) string {
	t.Helper()
	return reported(t, `comparguments -i '' : `+specs+`
		local -a n d od e
		comparguments -O n d od e
		local -a m=( ${n%%:*} )
		say "${m[*]}"`, line)
}

// The length of the name being shut off is what decides, and nothing about
// the word under the cursor.
func TestAnExclusionListWhileItsOwnOptionIsUnderTheCursor(t *testing.T) {
	for _, c := range []struct{ name, specs, line, want string }{
		// The idiom itself, both spellings. Typing either one leaves the
		// long one offered and the short one is the cursor's own, offered
		// back.
		{
			"the short spelling of a mutual pair",
			`'(-f --force)-f[force]' '(-f --force)--force[force]' '-p[proc]'`,
			"cmd -f", "-f --force -p",
		},
		{
			"the long spelling of the same pair",
			`'(-f --force)-f[force]' '(-f --force)--force[force]' '-p[proc]'`,
			"cmd --force", "--force -p",
		},
		// One keystroke later the list applies as written, which is the
		// control every other row here needs: without it, "the list is
		// ignored" would fit as well as the rule does.
		{
			"and once the word is finished",
			`'(-f --force)-f[force]' '(-f --force)--force[force]' '-p[proc]'`,
			"cmd -f ", "-p",
		},
		// A list naming its own option, alone.
		{
			"a list naming only itself",
			`'(-a)-a[all]' '-m[machine]' '-p[proc]'`,
			"cmd -a", "-a -m -p",
		},
		// A list naming somebody else: single letters go, longer names stay,
		// and the four rows below are the whole of the rule.
		{
			"a single-letter name is shut off",
			`'(-x --yy)-a[all]' '-x[ex]' '--yy[why]' '-p[proc]'`,
			"cmd -a", "-a --yy -p",
		},
		{
			"a two-letter one is not",
			`'(-x -xy)-a[all]' '-x[ex]' '-xy[exwhy]' '-p[proc]'`,
			"cmd -a", "-a -xy -p",
		},
		{
			"nor a longer short-style one, and a `+` name is",
			`'(-abc +z --yy)-a[all]' '-abc[three]' '+z[plus]' '--yy[why]' '-p[proc]'`,
			"cmd -a", "-a -abc --yy -p",
		},
		{
			// The cursor word being long changes nothing, which is what says
			// the rule is about the *name being shut off*.
			"a long option under the cursor shuts off the short ones",
			`'(--yy -x)--aa[aa]' '--yy[why]' '-x[ex]' '-p[proc]'`,
			"cmd --aa", "--aa --yy -p",
		},
		{
			"and so does a multi-letter short one",
			`'(-x --yy)-ab[ab]' '-x[ex]' '--yy[why]' '-p[proc]'`,
			"cmd -ab", "-ab --yy -p",
		},
		// `-` stands for every option and is filtered the same way.
		{
			"a list of `-` reaches only the single letters",
			`'(-)-a[all]' '-x[ex]' '--yy[why]' '-p[proc]'`,
			"cmd -a", "-a --yy",
		},
		// An ordinary exclusion, which must go on working: it is the same
		// mechanism and only the cursor's own list is narrowed.
		{
			"an ordinary exclusion still applies",
			`'(-m)-a[all]' '-m[machine]' '-p[proc]'`,
			"cmd -a", "-a -p",
		},
		{
			"a mutual pair of single letters",
			`'(-m)-a[all]' '(-a)-m[machine]' '-p[proc]'`,
			"cmd -a", "-a -p",
		},
		{
			// An option the cursor is not on shuts off as written, however
			// long the name.
			"a finished option shuts off a long name too",
			`'(-x --yy)-a[all]' '-x[ex]' '--yy[why]' '-p[proc]'`,
			"cmd -a ", "-p",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := offeredAt(t, c.specs, c.line); got != c.want {
				t.Errorf("%s over %q offered %q, want %q", c.specs, c.line, got, c.want)
			}
		})
	}
}
