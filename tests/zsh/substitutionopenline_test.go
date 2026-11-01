// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A command substitution's child carries the part of its parent's trace line
// that is not written yet, and three more of the lines it opens take that text
// the way an assignment list and a `case` arm already did (#6623). Measured
// 2026-10-30 against zsh 5.9.2 under `-f -c`, both streams on one pipe (#6711).
//
// A prefix's line starts with the inherited text, and a substitution of its own
// takes the text with it. A substitution run inside a compound command or a
// redirection takes it as well, where one in a plain command's word leaves it
// for the command's own line. A `[[ … ]]` is written as far as it got ahead of
// a substitution in an operand, and a pattern operand is expanded a second
// time for the line. And a prefix that stopped the child on an exit request
// is not written, where the inherited text goes out ahead of the complaint.
// See TestAChildWritesTheTextItWasHandedOnItsOwn for where the text goes out
// by itself.
func TestASubstitutionCarriesItsParentsOpenLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a prefix", `set -x; x=$( a=1 true )`, "+zsh:1> x=+zsh:1> a=1 +zsh:1> true\n+zsh:1> x='' \n"},
		{
			"a prefix to a program",
			`set -x; x=$( a=1 /usr/bin/true )`,
			"+zsh:1> x=+zsh:1> a=1 /usr/bin/true\n+zsh:1> x='' \n",
		},
		{
			"a substitution in a prefix",
			`set -x; x=$( a=$(echo s >&2) true )`,
			"+zsh:1> x=+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' +zsh:1> true\n+zsh:1> x='' \n",
		},
		{
			"two substitutions in a prefix",
			`set -x; x=$( a=$(echo s >&2) b=$(echo t >&2) true )`,
			"+zsh:1> x=+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' b=+zsh:1> echo t\nt\n" +
				"+zsh:1> a='' b='' +zsh:1> true\n+zsh:1> x='' \n",
		},
		{
			"a command after the prefix",
			`set -x; x=$( a=$(echo s >&2) true; echo u >&2 )`,
			"+zsh:1> x=+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' +zsh:1> true\n+zsh:1> echo u\nu\n+zsh:1> x='' \n",
		},
		{"a prefix stopped by a request", `set -x; x=$( a=1 b=${q?b} true )`, "+zsh:1> x=zsh:1: q: b\n+zsh:1> x='' \n"},
		{
			"a prefix stopped by a failure",
			`set -x; x=$( a=1 b=$((1/0)) true )`,
			"+zsh:1> x=zsh:1: division by zero\n+zsh:1> a=1 b=+zsh:1> x='' \n",
		},
		{
			"a for word list",
			`set -x; x=$( for i in $(echo a); do :; done )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> i=a\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"two substitutions in a for word list",
			`set -x; x=$( for i in $(echo a) $(echo b); do :; done )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> echo b\n+zsh:1> i=a\n+zsh:1> :\n+zsh:1> i=b\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a group",
			`set -x; x=$( { : $(echo a); } )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> : a\n+zsh:1> x='' \n",
		},
		{
			"a redirection",
			`set -x; x=$( : >$(echo /dev/null) )`,
			"+zsh:1> x=+zsh:1> echo /dev/null\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a pattern operand",
			`set -x; x=$( [[ a == $(echo a) ]] )`,
			"+zsh:1> x=+zsh:1> [[ a == +zsh:1> echo a\n+zsh:1> x=+zsh:1> [[ a == a+zsh:1> echo a\n" +
				"+zsh:1> x=+zsh:1> [[ a == a ]]\n+zsh:1> x='' \n",
		},
		{
			"a condition in an if",
			`set -x; x=$( if [[ $(echo a) == a ]]; then :; fi )`,
			"+zsh:1> x=+zsh:1> [[+zsh:1> echo a\n+zsh:1> [[ a == a ]]\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a pattern operand at the top",
			`set -x; [[ a == $(echo a) ]]`,
			"+zsh:1> [[ a == +zsh:1> echo a\n+zsh:1> [[ a == a+zsh:1> echo a\n+zsh:1> [[ a == a ]]\n",
		},
		{"a unary operand", `set -x; [[ -n $(echo a) ]]`, "+zsh:1> [[+zsh:1> echo a\n+zsh:1> [[ -n a ]]\n"},
		{
			"behind an and",
			`set -x; [[ -n a && -n $(echo b) ]]`,
			"+zsh:1> [[ -n a &&+zsh:1> echo b\n+zsh:1> [[ -n a && -n b ]]\n",
		},
		{"the pattern expanded twice", `n=0; set -x; [[ a == $((n++)) ]]; echo $n`, "+zsh:1> [[ a == 0 ]]\n+zsh:1> echo 2\n2\n"},
		{
			"the line holds the first and the match the second",
			`n=0; set -x; [[ 2 == $((++n)) ]]; echo st=$? n=$n`,
			"+zsh:1> [[ 2 == 1 ]]\n+zsh:1> echo 'st=0' 'n=2'\nst=0 n=2\n",
		},
		// Controls.
		{
			"a plain command's word",
			`set -x; x=$( : $(echo a); : )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> x=+zsh:1> : a\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{"a for with no substitution", `set -x; x=$( for i in a; do :; done )`, "+zsh:1> x=+zsh:1> i=a\n+zsh:1> :\n+zsh:1> x='' \n"},
		{"untraced", `n=0; [[ a == $((n++)) ]]; echo $n`, "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// Three places write the text a child was handed on its own, ahead of
// whatever comes next: entering a loop, an `if`, a subshell, `repeat` or
// `time`; opening a redirection, before it is made; and, in a group or a
// `try`, a substitution in a command's words. So the text is not lost when
// what follows writes nothing (`f() { :; }` is a body that writes nothing),
// and it reaches the stream a redirection is about to move. Measured
// 2026-10-30 against zsh 5.9.2 under `-f -c`, both streams on one pipe
// (#6711).
func TestAChildWritesTheTextItWasHandedOnItsOwn(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"standard error redirected", `set -x; x=$( { echo hi } 2>/dev/null )`, "+zsh:1> x=+zsh:1> x=hi \n"},
		{
			"standard error redirected and the child stopped",
			`set -x; x=$( { : ${q?b} } 2>/dev/null; echo y )`,
			"+zsh:1> x=+zsh:1> x='' \n",
		},
		{
			"standard error onto the output",
			`set -x; x=$( { echo hi } 2>&1 )`,
			"+zsh:1> x=+zsh:1> x=$'+zsh:1> echo hi\\nhi' \n",
		},
		{"another descriptor redirected", `set -x; x=$( { } 3>/dev/null )`, "+zsh:1> x=+zsh:1> x='' \n"},
		{"a definition with a redirection", `set -x; x=$( f() { :; } 2>/dev/null )`, "+zsh:1> x=+zsh:1> x='' \n"},
		{"a loop that runs nothing", `set -x; x=$( for i in; do :; done )`, "+zsh:1> x=+zsh:1> x='' \n"},
		{"an if that writes nothing", `set -x; x=$( if ! { }; then :; fi )`, "+zsh:1> x=+zsh:1> x='' \n"},
		{"a subshell that writes nothing", `set -x; x=$( ( f() { :; } ) )`, "+zsh:1> x=+zsh:1> x='' \n"},
		{"a timed group that writes nothing", `set -x; x=$( time { f() { :; } } )`, "+zsh:1> x=+zsh:1> x='' \n"},
		{
			"a quiet substitution in a group",
			`set -x; x=$( { : $(f() { :; }); } )`,
			"+zsh:1> x=+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a quiet substitution in a prefix",
			`set -x; x=$( a=$(f() { :; }) true )`,
			"+zsh:1> x=+zsh:1> a='' +zsh:1> true\n+zsh:1> x='' \n",
		},
		{
			"the always half of a try",
			`set -x; x=$( { f() { :; } } always { : $(echo a) } )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> : a\n+zsh:1> x='' \n",
		},
		{
			"a condition in a group carries it",
			`set -x; x=$( { [[ a == $(echo a) ]]; } )`,
			"+zsh:1> x=+zsh:1> [[ a == +zsh:1> echo a\n+zsh:1> x=+zsh:1> [[ a == a+zsh:1> echo a\n" +
				"+zsh:1> x=+zsh:1> [[ a == a ]]\n+zsh:1> x='' \n",
		},
		// Controls: a group that writes nothing drops it, as a definition
		// does, and what a group does ends with the group.
		{"a group that writes nothing", `set -x; x=$( { f() { :; } } )`, "+zsh:1> x='' \n"},
		{"a definition", `set -x; x=$( f() { :; } )`, "+zsh:1> x='' \n"},
		{
			"a word after a group",
			`set -x; x=$( { f() { :; } }; : $(echo a); : )`,
			"+zsh:1> x=+zsh:1> echo a\n+zsh:1> x=+zsh:1> : a\n+zsh:1> :\n+zsh:1> x='' \n",
		},
		{
			"a quiet substitution at the top",
			`set -x; x=$( : $(f() { :; }) )`,
			"+zsh:1> x=+zsh:1> :\n+zsh:1> x='' \n",
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

// A subscripted prefix entry's subscript is read once its value is in, and
// the line a substitution in the subscript copies, or a failing subscript
// leaves, has the value on it. Measured 2026-10-30 against zsh 5.9.2 under
// `-f -c`, both streams on one pipe (#6711).
func TestASubscriptIsReadBehindItsEntrysValue(t *testing.T) {
	t.Parallel()
	const fn = `f() { echo "f y=$y"; }; b=(x y z); set -x; `
	for _, c := range []struct{ name, src, want string }{
		{
			"a substitution in the subscript",
			fn + `b[$(echo s >&2; echo 1)]=$(echo t >&2; echo v) y=$(echo u >&2)${b[1]} f`,
			"+zsh:1> b[$(echo s >&2; echo 1)]=+zsh:1> echo t\nt\n+zsh:1> echo v\n" +
				"+zsh:1> b[$(echo s >&2; echo 1)]=v +zsh:1> echo s\ns\n+zsh:1> echo 1\n" +
				"+zsh:1> b[$(echo s >&2; echo 1)]=v y=+zsh:1> echo u\nu\n" +
				"+zsh:1> b[$(echo s >&2; echo 1)]=v y=v +zsh:1> f\n+f:0> echo 'f y=v'\nf y=v\n",
		},
		{"a failing subscript", fn + `b[1/0]=v y=1 f`, "zsh:1: division by zero\n+zsh:1> b[1/0]=v "},
		{"behind another entry", fn + `a=1 b[1/0]=v y=1 f`, "zsh:1: division by zero\n+zsh:1> a=1 b[1/0]=v "},
		{"a refused subscript", fn + `b[${q?e}]=v y=1 f`, "zsh:1: q: e\n+zsh:1> b[${q?e}]=v "},
		// The control: a failing value leaves the line at the name.
		{"a failing value", fn + `b[1]=$((1/0)) y=1 f`, "zsh:1: division by zero\n+zsh:1> b[1]="},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
