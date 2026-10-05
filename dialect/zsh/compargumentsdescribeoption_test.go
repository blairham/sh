// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"path/filepath"
	"testing"
)

// What `_arguments` needs from `comparguments` once the word under the cursor
// is an option written with its argument — `ls --color=<TAB>` — and the
// read-back verbs' arity, which zsh checks and this did not (#6165).
//
// Every row was measured on zsh 5.9.2, 2026-10-05 through a pseudo-terminal
// from inside a real `zle -C` widget, against one spec set holding every form
// an option's argument can take. The measurements are in comparguments.go and
// compargumentsline.go beside the code that answers them.

const describeOptionSpecs = `'-a[all]' '-f+[file]:fname:_files' ` +
	`'--color=-[col]:color:(never always)' '--opt=[o]:oo:(o1)' ` +
	`'-x-[dir]:xx:(x1)' '-n[n]:nn:(n1)' '-o=[oo]:ooo:(o1)' ` +
	`'-m[multi]:first:(x y):second:(p q)' '*:rest:(r1)'`

// TestCompargumentsLDescribesAnOptionsFirstArgument is `-L`, which was refused
// as an invalid option — and `_arguments` printed that refusal over the line.
func TestCompargumentsLDescribesAnOptionsFirstArgument(t *testing.T) {
	for _, c := range []struct{ name, option, want string }{
		{"an argument that attaches or follows", "-f", "0 fname|_files|option-f-1"},
		{"one after an `=` only", "--color", "0 color|(never always)|option--color-1"},
		{"one after an `=` or the next word", "--opt", "0 oo|(o1)|option--opt-1"},
		{"one that must attach", "-x", "0 xx|(x1)|option-x-1"},
		{"the first of two", "-m", "0 first|(x y)|option-m-1"},
		// Status 1, and the three names keep what the caller had in them.
		{"an option with no argument", "-a", "1 D0|A0|S0"},
		{"an option nobody declared", "-zz", "1 D0|A0|S0"},
		{"the name must be exact", "--color=", "1 D0|A0|S0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := reported(t, `comparguments -i '' : `+describeOptionSpecs+`
				local -a d=(D0) a=(A0) s=(S0)
				comparguments -L `+c.option+` d a s
				say "$? ${d[*]}|${a[*]}|${s[*]}"`, "cmd ")
			if got != c.want {
				t.Errorf("comparguments -L %s: got %q, want %q", c.option, got, c.want)
			}
		})
	}
}

// TestTheReadBackVerbsTakeAnExactCount is the arity zsh checks before any of
// them answers: one word fewer is `not enough arguments`, one more is `too
// many arguments`, and either way nothing is assigned.
func TestTheReadBackVerbsTakeAnExactCount(t *testing.T) {
	for _, c := range []struct{ name, call, want string }{
		{"-D with four", "-D a b c d", "1 too many arguments"},
		{"-O with five", "-O a b c d e", "1 too many arguments"},
		{"-W with four", "-W a b c d", "1 too many arguments"},
		{"-M with two", "-M a b", "1 too many arguments"},
		{"-s with two", "-s a b", "1 too many arguments"},
		{"-a with one", "-a a", "1 too many arguments"},
		{"-L with five", "-L -f a b c d", "1 too many arguments"},
		{"-L with two", "-L -f a", "1 not enough arguments"},
		{"-L with one", "-L -f", "1 not enough arguments"},
		{"-D with two", "-D a b", "1 not enough arguments"},
	} {
		t.Run(c.name, func(t *testing.T) {
			errs := filepath.Join(t.TempDir(), "err")
			got := reported(t, `comparguments -i '' : `+describeOptionSpecs+`
				local a=A b=B c=C d=D e=E
				comparguments `+c.call+` 2>`+errs+`
				local st=$?
				say "$st ${$(<`+errs+`)##*: }"
				[[ $a$b$c$d$e == ABCDE ]] || say "assigned: $a $b $c $d $e"`, "cmd ")
			if got != c.want {
				t.Errorf("comparguments %s: got %q, want %q", c.call, got, c.want)
			}
		})
	}
}

// TestAReadBackVerbThatAnswersNonZeroAssignsNothing — the names keep the
// caller's values. This emptied them.
func TestAReadBackVerbThatAnswersNonZeroAssignsNothing(t *testing.T) {
	for _, c := range []struct{ name, specs, line, call, want string }{
		{
			"-D with nothing to describe", describeOptionSpecs, "cmd -a",
			"-D p q r", "1 P|Q|R|S",
		},
		{
			"-O once an argument shut the options off", `'-v[x]' '(-)1:first:(a b)'`,
			"cmd a -", "-O p q r s", "1 P|Q|R|S",
		},
		{
			"-s with no stack under the cursor", describeOptionSpecs, "cmd --color",
			"-s p", "1 P|Q|R|S",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := reported(t, `comparguments -i '' -s : `+c.specs+`
				local -a p=(P) q=(Q) r=(R) s=(S)
				comparguments `+c.call+`
				say "$? ${p[*]}|${q[*]}|${r[*]}|${s[*]}"`, c.line)
			if got != c.want {
				t.Errorf("%s on %q: got %q, want %q", c.call, c.line, got, c.want)
			}
		})
	}
}

// TestAnOptionWrittenWithItsEqualsIsOfferedBack is `-O` while the cursor is
// after an option's `=`: that option is still in the `equal` array, which is
// what sends `_arguments` on to `-L` — where its name is longer than one
// letter. The two forms whose argument attaches without an `=` are spent, as
// before.
func TestAnOptionWrittenWithItsEqualsIsOfferedBack(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"cmd --color=", "-x:dir|-f:file|--color:col --opt:o -o:oo"},
		{"cmd --color=al", "-x:dir|-f:file|--color:col --opt:o -o:oo"},
		{"cmd --opt=o", "-x:dir|-f:file|--color:col --opt:o -o:oo"},
		// A one-letter name is spent even after its `=`.
		{"cmd -o=v", "-x:dir|-f:file|--color:col --opt:o"},
		{"cmd -fx", "-x:dir||--color:col --opt:o -o:oo"},
		{"cmd -xq", "|-f:file|--color:col --opt:o -o:oo"},
	} {
		t.Run(c.line, func(t *testing.T) {
			got := reported(t, `comparguments -i '' -s : `+describeOptionSpecs+`
				local -a n d o e
				comparguments -O n d o e
				say "${d[*]}|${o[*]}|${e[*]}"`, c.line)
			if got != c.want {
				t.Errorf("-O on %q: got %q, want %q", c.line, got, c.want)
			}
		})
	}
}

// TestDescribingAnAttachedArgumentMovesTheOptionIntoIPREFIX is `-D`'s other
// half: where the argument shares its word with the option, the option's part
// leaves `$PREFIX` for `$IPREFIX`, so the action's words are matched against
// the argument alone. Without it `ls --color=<TAB>` listed nothing.
func TestDescribingAnAttachedArgumentMovesTheOptionIntoIPREFIX(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{"cmd --color=", "--color=|"},
		{"cmd --color=al", "--color=|al"},
		{"cmd --opt=o", "--opt=|o"},
		{"cmd -o=v", "-o=|v"},
		{"cmd -fx", "-f|x"},
		{"cmd -f", "-f|"},
		{"cmd -xq", "-x|q"},
		{"cmd -af", "-af|"},
		// The controls: an argument that is its own word, and a normal one.
		{"cmd -n v", "|v"},
		{"cmd plain", "|plain"},
	} {
		t.Run(c.line, func(t *testing.T) {
			got := reported(t, `comparguments -i '' -s : `+describeOptionSpecs+`
				local -a d a s
				comparguments -D d a s
				local moved="$IPREFIX|$PREFIX"
				IPREFIX= PREFIX=
				say "$moved"`, c.line)
			if got != c.want {
				t.Errorf("-D on %q: IPREFIX|PREFIX = %q, want %q", c.line, got, c.want)
			}
		})
	}
}
