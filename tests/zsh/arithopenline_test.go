// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An arithmetic command holds its trace line open as `((` while its expression
// expands, so a substitution in it writes its own line behind that text, as a
// `[[ … ]]` operand's does (#6711). Measured 2026-11-02 against zsh 5.9.2 under
// `-f -c`, both streams on one pipe (#7108).
func TestAnArithmeticCommandHoldsItsLineOpen(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a substitution", `set -x; (( $(echo 1) ))`, "+zsh:1> ((+zsh:1> echo 1\n+zsh:1> ((  1  ))\n"},
		{
			"two substitutions",
			`set -x; (( $(echo 1) + $(echo 2) ))`,
			"+zsh:1> ((+zsh:1> echo 1\n+zsh:1> ((+zsh:1> echo 2\n+zsh:1> ((  1 + 2  ))\n",
		},
		{
			"inside a substitution",
			`set -x; x=$( (( $(echo 1) )) )`,
			"+zsh:1> x=+zsh:1> ((+zsh:1> echo 1\n+zsh:1> x=+zsh:1> ((  1  ))\n+zsh:1> x='' \n",
		},
		{
			"in a group inside a substitution",
			`set -x; x=$( { (( $(echo 1) )); } )`,
			"+zsh:1> x=+zsh:1> ((+zsh:1> echo 1\n+zsh:1> x=+zsh:1> ((  1  ))\n+zsh:1> x='' \n",
		},
		{
			"in a function",
			`set -x; f() { (( $(echo 1) )); }; f`,
			"+zsh:1> f\n+f:0> ((+f:0> echo 1\n+f:0> ((  1  ))\n",
		},
		{
			"a substitution writing to standard error",
			`set -x; (( $(echo 1 >&2; echo 2) ))`,
			"+zsh:1> ((+zsh:1> echo 1\n1\n+zsh:1> echo 2\n+zsh:1> ((  2  ))\n",
		},
		{
			"a failure in the expression",
			`set -x; (( $((1/0)) )); echo st=$?`,
			"+zsh:1> ((zsh:1: division by zero\n   ))\n+zsh:1> echo 'st=2'\nst=2\n",
		},
		{"a request to stop", `set -x; (( ${q?b} )); echo st=$?`, "+zsh:1> ((zsh:1: q: b\n"},
		{
			"a request to stop inside a substitution",
			`set -x; x=$( (( ${q?b} )) ); echo st=$?`,
			"+zsh:1> x=+zsh:1> ((zsh:1: q: b\n+zsh:1> x='' \n+zsh:1> echo 'st=1'\nst=1\n",
		},
		// Controls.
		{"no substitution", `set -x; (( 1 ))`, "+zsh:1> ((  1  ))\n"},
		{"a substitution that writes nothing", `set -x; (( $(f() { :; }) 1 ))`, "+zsh:1> ((   1  ))\n"},
		{"untraced", `(( $(echo 1) )); echo st=$?`, "st=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// A declaration's operand written as an assignment, at the top of a
// substitution's body, writes the text the body was handed before a
// substitution in it starts, and the declaration's own line goes out without
// it, as a command in a group does (#6711). A plain word of the same command
// leaves it for the substitution's copy, as any command's words do. Measured
// 2026-11-02 against zsh 5.9.2 under `-f -c`, both streams on one pipe (#7118).
func TestADeclarationsOperandHandsOnTheTextItWasHanded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"typeset",
			`set -x; x=$( typeset y=$(echo a) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> typeset y=a\n+zsh:1> x='' \n",
		},
		{
			"local",
			`set -x; x=$( local y=$(echo a) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> local y=a\n+zsh:1> x='' \n",
		},
		{
			"export",
			`set -x; x=$( export y=$(echo a) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> export y=a\n+zsh:1> x='' \n",
		},
		{
			"an operand behind one without a substitution",
			`set -x; x=$( typeset y=1 z=$(echo a) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> typeset y=1 z=a\n+zsh:1> x='' \n",
		},
		{
			"an array literal",
			`set -x; x=$( typeset -a y=( $(echo a) ) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> typeset -a y=( a )\n+zsh:1> x='' \n",
		},
		{
			"a plain word in front of the operand",
			`set -x; x=$( typeset $(echo y)=a z=$(echo b) )`,
			"+zsh:1> x=+zsh:1> echo y\n+zsh:1> x=+zsh:1> echo b\n+zsh:1> typeset 'y=a' z=b\n+zsh:1> x='' \n",
		},
		{
			"a plain word behind the operand",
			`set -x; x=$( typeset z=$(echo b) $(echo y)=a )`,
			"+zsh:1> x=+zsh:1> echo b\n+zsh:1> echo y\n+zsh:1> typeset z=b y=a\n+zsh:1> x='' \n",
		},
		{
			"a command after it",
			`set -x; x=$( typeset y=$(echo a); : )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> typeset y=a\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		// Controls.
		{
			"a plain command's word",
			`set -x; x=$( : $(echo a) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> x=+zsh:1> : a\n+zsh:1> x='' \n",
		},
		{
			"a declaration in a group",
			`set -x; x=$( { typeset y=$(echo a); } )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> typeset y=a\n+zsh:1> x='' \n",
		},
		{"no substitution", `set -x; x=$( typeset y=a )`, "+zsh:1> x=+zsh:1> typeset y=a\n+zsh:1> x='' \n"},
		{
			"behind builtin",
			`set -x; x=$( builtin typeset y=$(echo a) )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> x=+zsh:1> typeset 'y=a'\n+zsh:1> x='' \n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// An arithmetic `for` traces an empty initializer, and an empty step each time
// round, as a line holding the prefix alone, and an empty condition not at all.
// Measured 2026-11-02 against zsh 5.9.2 under `-f -c` (#7117).
func TestAnEmptyArithForInitializerOrStepIsTraced(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"an empty initializer", `set -x; for ((;0;)) do :; done`, "+zsh:1> \n+zsh:1> 0\n"},
		{"everything empty", `set -x; for ((;;)) do break; done`, "+zsh:1> \n+zsh:1> break\n"},
		{"blanks", `set -x; for ((  ;0;  )) do :; done`, "+zsh:1> \n+zsh:1> 0\n"},
		{
			"an empty step",
			`set -x; for ((i=0;i<2;)) do (( i++ )); done`,
			"+zsh:1> i=0\n+zsh:1> i<2\n+zsh:1> ((  i++  ))\n+zsh:1> \n" +
				"+zsh:1> i<2\n+zsh:1> ((  i++  ))\n+zsh:1> \n+zsh:1> i<2\n",
		},
		{
			"inside a substitution",
			`set -x; x=$( for ((;0;)) do :; done )`,
			"+zsh:1> x=+zsh:1> \n+zsh:1> 0\n+zsh:1> x='' \n",
		},
		// Controls.
		{"an empty condition", `set -x; for ((i=0;;)) do break; done`, "+zsh:1> i=0\n+zsh:1> break\n"},
		{"nothing empty", `set -x; for ((i=0;i<0;)) do :; done`, "+zsh:1> i=0\n+zsh:1> i<0\n"},
		{"untraced", `for ((;0;)) do :; done; echo ok`, "ok\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
