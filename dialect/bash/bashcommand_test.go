// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `$BASH_COMMAND`, the command this shell is running written back out. Every
// row was run in bash 5.3.15 and in this shell before it was written down —
// 2026-09-14, `env -i PATH=/usr/bin:/bin`, over `-c` — and the whole table
// was empty here before #2779.
//
// The action reads the parameter **quoted** throughout. Unquoted it is split
// and rejoined by the `echo` that prints it, which collapses every run of
// spaces — so an unquoted probe cannot tell a shell that keeps the script's
// spacing from one that does not, and the first reading of this behavior was
// taken from exactly that probe.

// TestADebugActionNamesTheCommandItFiredFor is what the parameter is for: an
// action that cannot tell one command from another can only act on all of
// them, so a breakpoint is not expressible without this.
func TestADebugActionNamesTheCommandItFiredFor(t *testing.T) {
	const act = `trap 'echo "D:[$BASH_COMMAND]"' DEBUG; `
	for _, c := range []struct {
		name, src, want string
	}{
		{"each simple command in turn", act + `echo one; echo two`, "D:[echo one]\none\nD:[echo two]\ntwo\n"},
		// The words keep their quoting and their expansions as the script
		// wrote them — it is the *unexpanded* command, which is what makes it
		// something `set -x` cannot stand in for — and the spacing between
		// them is this shell's rather than the script's.
		{"an assignment and a quoted word", act + `x=1 echo $x "a  b"`, `D:[x=1 echo $x "a  b"]` + "\na  b\n"},
		{"spacing is the shell's", act + `echo    one`, "D:[echo one]\none\n"},
		{"a redirection is part of the command", act + `echo hi   >   /dev/null`, "D:[echo hi > /dev/null]\n"},
		{"a herestring is too", act + `cat <<<hi`, "D:[cat <<< hi]\nhi\n"},
		// A call is named by the call, and the body's commands by
		// themselves; `set -T` is what lets the trap into the body at all,
		// and the second `g` is that option's refire once the frame has been
		// entered — with the call word still the command being run.
		{"a call and then its body", `g(){ echo g; }; set -T; ` + act + `g`, "D:[g]\nD:[g]\nD:[echo g]\ng\n"},
		// A function *definition* fires nothing and names nothing, which is
		// the control for the row above: both of its `g` lines belong to the
		// call.
		{"a definition fires nothing", act + `g(){ echo g; }; echo after`, "D:[echo after]\nafter\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestACompoundHeadIsNamedAsItsHead is the half a simple-command-only reading
// gets wrong: a head that fires is named as the head and not as the construct
// it opens, and this shell fills in two things the script left out.
func TestACompoundHeadIsNamedAsItsHead(t *testing.T) {
	const act = `trap 'echo "D:[$BASH_COMMAND]"' DEBUG; `
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"a list loop names its head once per pass", act + `for w in a b; do :; done`,
			"D:[for w in a b]\nD:[:]\nD:[for w in a b]\nD:[:]\n",
		},
		{
			"and the head loses the script's spacing", act + `for  w  in  a   b; do :; done`,
			"D:[for w in a b]\nD:[:]\nD:[for w in a b]\nD:[:]\n",
		},
		// A loop with no list reads as though it had written the one it
		// takes, which is this shell saying what the loop means rather than
		// what the script typed.
		{
			"a loop with no list reads as the parameters", `set -- p; ` + act + `for w; do :; done`,
			`D:[for w in "$@"]` + "\nD:[:]\n",
		},
		// The trailing space is this shell's: a head printed with no arms
		// after it.
		{
			"a case head keeps its trailing space", act + `case "x y" in *) :;; esac`,
			`D:[case "x y" in ]` + "\nD:[:]\n",
		},
		{"a test head is the whole of it", act + `[[ 1 ==  1 ]]`, "D:[[[ 1 == 1 ]]]\n"},
		{"an arithmetic command keeps its own text", act + `((1+1))`, "D:[((1+1))]\n"},
		// The three expressions of an arithmetic loop are each named on
		// their own, every time one is evaluated.
		{
			"an arithmetic loop names its three parts", act + `for ((i=0;i<1;i++)); do :; done`,
			"D:[((i=0))]\nD:[((i<1))]\nD:[:]\nD:[((i++))]\nD:[((i<1))]\n",
		},
		// And a part the script left out reads as the value it stands for.
		{
			"a part left out reads as 1", act + `for ((;;)); do break; done`,
			"D:[((1))]\nD:[((1))]\nD:[break]\n",
		},
		// The heads that fire nothing name nothing either, which is what
		// says the record follows the firing sites rather than every node.
		{"an if fires no head", act + `if true; then echo y; fi`, "D:[true]\nD:[echo y]\ny\n"},
		{"a group fires none", act + `{ echo a; }`, "D:[echo a]\na\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestTheRunningCommandIsRecordedWithNoTrapSet is the rule that makes the
// parameter a fact about the shell rather than about the trap: it is written
// before each command's own words are expanded, so a command reading it reads
// itself, and a trap body writes nothing to it at all.
func TestTheRunningCommandIsRecordedWithNoTrapSet(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{"a command reading it reads itself", `echo "[$BASH_COMMAND]"`, `[echo "[$BASH_COMMAND]"]` + "\n"},
		{"and still does after another command ran", `true; echo "[$BASH_COMMAND]"`, `[echo "[$BASH_COMMAND]"]` + "\n"},
		// The head of a list loop is recorded *after* its word list has been
		// expanded, so the expansion still reads the command before it —
		// which is the one head that can be told apart this way.
		{
			"a list loop's head is recorded after its list is expanded",
			`true; for w in "$BASH_COMMAND"; do echo "[$w]"; done`, "[true]\n",
		},
		// Where a `case` head is recorded before its word is expanded, so
		// its own text is what the word reads.
		{
			"a case head is recorded before its word is expanded",
			`true; case "$BASH_COMMAND" in true) echo unmoved;; *) echo "[$BASH_COMMAND]";; esac`,
			`[echo "[$BASH_COMMAND]"]` + "\n",
		},
		// An action runs a command of its own and still names the one it
		// fired for, which is the whole of what makes it usable.
		{"a trap body moves nothing", `trap 'true; echo "D:[$BASH_COMMAND]"' DEBUG; echo one`, "D:[echo one]\none\n"},
		// Nor does a function the body calls, so the suspension is the body
		// and everything under it rather than one command deep.
		{
			"nor does a function the body calls",
			`f(){ echo inner; }; trap 'f; echo "D:[$BASH_COMMAND]"' DEBUG; echo one`,
			"inner\nD:[echo one]\none\n",
		},
		// And it reaches every trap body and not only DEBUG's, which is what
		// lets an EXIT action say where the script had got to.
		{
			"an exit action names the command the script reached",
			`trap 'echo "X:[$BASH_COMMAND]"' EXIT; echo one`, "one\nX:[echo one]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}

// TestAssigningToTheRunningCommandDoesNothing is the writer, and without one
// the assignment would land in the stored table — which answers ahead of a
// producer, so the parameter would stop tracking the shell from the moment a
// script touched it.
func TestAssigningToTheRunningCommandDoesNothing(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"an assignment is overwritten by the next command",
			`BASH_COMMAND=zzz; echo "[$BASH_COMMAND]"`, `[echo "[$BASH_COMMAND]"]` + "\n",
		},
		{
			"and it lists with no attribute", `declare -p BASH_COMMAND`,
			`declare -- BASH_COMMAND="declare -p BASH_COMMAND"` + "\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runTraced(t, c.src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and nothing said",
					c.src, out, errs, code, c.want)
			}
		})
	}
}
