// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// What `set -x` writes for an assignment whose value fails: where the line
// stops, whether it gets its newline, and what a substitution in the value
// writes of its parent's line (#6623).
//
// Measured 2026-10-15 on zsh 5.9.2 under `-f -c`, with both streams on one
// pipe and read through `sed -n l`. The line is ended after the complaint
// unless the failure is a `${x?word}` the shell exits on, an array literal
// writes nothing of itself when an element fails, and a substitution writes
// its copy of the parent's line ahead of its complaint as it does ahead of its
// first trace line (except under a command's prefix, as in the parent).
func TestATracedAssignmentThatFailsEndsItsLine(t *testing.T) {
	rows := []struct{ src, errs string }{
		{`set -x; x=${q?b}`, "+zsh:1> x=zsh:1: q: b\n"},
		{`set -x; set -u; x=$nope`, "+zsh:1> set -u\n+zsh:1> x=zsh:1: nope: parameter not set\n\n"},
		{`set -x; x=$((1/0))`, "+zsh:1> x=zsh:1: division by zero\n\n"},
		{`set -x; x=( ${q?b} )`, "+zsh:1> x=zsh:1: q: b\n"},
		{`set -x; x=( a $((1/0)) b )`, "+zsh:1> x=zsh:1: division by zero\n\n"},
		{`set -x; x=$( echo ${q?b} )`, "+zsh:1> x=zsh:1: q: b\n+zsh:1> x='' \n"},
		{`set -x; a=1 x=$( echo ${q?b} ) b=2`, "+zsh:1> a=1 x=zsh:1: q: b\n+zsh:1> a=1 x='' b=2 \n"},
		{`set -x; x=( a $( echo ${q?b} ) )`, "+zsh:1> x=zsh:1: q: b\n+zsh:1> x=( a ) \n"},
		{`set -x; x=$( y=$( echo ${q?b} ) )`, "+zsh:1> x=+zsh:1> y=zsh:1: q: b\n+zsh:1> x=+zsh:1> y='' \n+zsh:1> x='' \n"},
		{`set -x; case a in b|$( echo $((1/0)) )) echo m ;; esac`, "+zsh:1> case a (bzsh:1: division by zero\n+zsh:1> case a (b | )\n"},
		// A `case` arm inside a substitution carries the parent's line in
		// front of its own, and still writes it in front of the whole line.
		{`set -x; x=$( case a in $(echo a)) ;; esac )`, "+zsh:1> x=+zsh:1> case a (+zsh:1> echo a\n+zsh:1> x=+zsh:1> case a (a)\n+zsh:1> x='' \n"},
		{`set -x; x=$( case a in $((1/0))) ;; esac )`, "+zsh:1> x=+zsh:1> case a (zsh:1: division by zero\n)\n+zsh:1> x='' \n"},
		{`set -x; x=$( case a in b|$( echo $((1/0)) )) ;; esac )`, "+zsh:1> x=+zsh:1> case a (bzsh:1: division by zero\n+zsh:1> x=+zsh:1> case a (b | )\n+zsh:1> x='' \n"},
		// Controls: the prefix line is written after the complaint, and a
		// child that traces a line writes its copy there, as before.
		{`set -x; x=$( echo ${q?b} ) true`, "zsh:1: q: b\n+zsh:1> x='' +zsh:1> true\n"},
		{`set -x; x=$( :; echo ${q?b} )`, "+zsh:1> x=+zsh:1> :\nzsh:1: q: b\n+zsh:1> x='' \n"},
	}
	t.Setenv("HOME", t.TempDir())
	for _, c := range rows {
		_, errs, _ := runZsh(t, "-f", "-c", c.src)
		if errs != c.errs {
			t.Errorf("%s\n got %q\nwant %q", c.src, errs, c.errs)
		}
	}
}

// The same at a prompt, where `${x?word}` is not a request to stop and the
// line is ended like any other: measured 2026-10-15 on zsh 5.9.2 run `-f -i`
// through a terminal and from standard input, the issue's two rows (#6623).
func TestATracedAssignmentThatFailsEndsItsLineAtAPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, errs, _ := prompt(t, "PS1=\nset -x\nx=${q?b}\nx=$( echo ${q?b} )\n", "zsh", "-f", "-i")
	for _, want := range []string{
		"+zsh:3> x=zsh: q: b\n\n",
		"+zsh:4> x=zsh: q: b\n+zsh:4> x='' \n",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr %q\nhas no %q", errs, want)
		}
	}
}
