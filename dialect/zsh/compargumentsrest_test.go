// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The specification set every case below is asked against: one option, one
// option that takes a word, and a rest specification whose colon count the
// case names. It is the shape the shipped `_git` uses at its top level.
const restSpecs = `'-v[verbose]' '-o[opt]:val:' `

// TestARestSpecificationMovesTheWords is what `git checkout <TAB>` needed and
// did not have.
//
// `_arguments` never touches `$words` or `$CURRENT`; `comparguments -D`
// replaces them, and the replacement outlives the call, because the completer
// the rest specification names reads `$words[1]` to decide what to do. The
// shipped `_git` ends its top-level specification with `(-)*:: :->option-or-
// argument` and its arm calls `_git-$words[1]`. With the words left as the
// line has them that is `_git-git`, which does not exist — so `git checkout
// <TAB>` fell back to `_default` and offered *files* where a person expected
// branches.
//
// Every row was measured on zsh 5.9.2, 2026-09-16 through a pseudo-terminal,
// from inside a `zle -C` widget whose function calls the builtin itself and
// prints `$words` and `$CURRENT` back.
func TestARestSpecificationMovesTheWords(t *testing.T) {
	for _, c := range []struct{ name, spec, line, want string }{
		// The rest specification with two colons replaces them; the same one
		// with a single colon leaves them alone. Both sides, because one
		// alone does not separate the rule from a shell that always replaces
		// or never does.
		{"two colons replace", `'*:: :->rest'`, "cmd sub ", "0/sub,/2"},
		{"one colon does not", `'*: :->rest'`, "cmd sub ", "0/cmd,sub,/3"},
		{"three colons replace", `'*::: :->rest'`, "cmd sub ", "0/sub,/2"},
		// A numbered optional argument is not the rest, and leaves them
		// alone — so it is the *rest* that moves them, not the `::`.
		{"a numbered :: does not", `'1:first:(a b)' '2::second:->s'`, "cmd sub ", "0/cmd,sub,/3"},
		// What they are replaced with is the normal arguments: options and
		// the words they take are gone, and the word under the cursor is the
		// last element.
		{"options are dropped", `'*:: :->rest'`, "cmd -o val sub arg ", "0/sub,arg,/3"},
		{"arguments are kept", `'*:: :->rest'`, "cmd a b c ", "0/a,b,c,/4"},
		{"nothing written yet", `'*:: :->rest'`, "cmd ", "0//1"},
		// **Once the rest specification has taken a word, options stop being
		// read at all** — the words belong to the sub-command now. Measured:
		// `cmd sub -o val <TAB>` keeps all four.
		{"a later option is a word", `'*:: :->rest'`, "cmd sub -o val ", "0/sub,-o,val,/4"},
		{"and so is the one being typed", `'*:: :->rest'`, "cmd sub -v", "0/sub,-v/2"},
		// But not before it has: at the first position nothing has been
		// written for a sub-command to own, so `-v` is still an option and
		// no argument specification applies at all.
		{"an option at the first position", `'*:: :->rest'`, "cmd -v", "1/cmd,-v/2"},
		{"a stack of them counts too", `'*:: :->rest'`, "cmd -vv", "1/cmd,-vv/2"},
		// A word that only looks like one is an argument, and starts the
		// rest. A lone `-`, a `--` and an option nobody declared are the
		// three shapes.
		{"a lone dash is an argument", `'*:: :->rest'`, "cmd -", "0/-/1"},
		{"a double dash is too", `'*:: :->rest'`, "cmd --", "0/--/1"},
		{"and an undeclared option", `'*:: :->rest'`, "cmd -x", "0/-x/1"},
		// A numbered specification in front of the rest delays it: the rest
		// has not begun until a word lands on a position it covers.
		{"a numbered spec delays it", `'1:first:(x y)' '*:: :->rest'`, "cmd a -v", "1/cmd,a,-v/3"},
		{"and then it begins", `'1:first:(x y)' '*:: :->rest'`, "cmd a b -v", "0/a,b,-v/3"},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := `comparguments -i '' -s : ` + restSpecs + c.spec + `
				local -a ds as ss
				comparguments -D ds as ss
				say "$?/${(j:,:)words}/$CURRENT"`
			if got := reported(t, body, c.line); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestTheOptionsStillOfferedAreTheOnesLeft is `-O`'s status and the two rules
// that make it mean something.
//
// `_arguments` reads the *status* rather than the four arrays, so a 0 with
// four empty arrays is a shipped completion told the options were handled.
// That is what stopped `git checkout -<TAB>` before it reached
// `_git-checkout`, and it was reached from two directions at once: everything
// excluded, and nothing offered.
//
// Measured on zsh 5.9.2, 2026-09-16 from inside a widget.
func TestTheOptionsStillOfferedAreTheOnesLeft(t *testing.T) {
	for _, c := range []struct{ name, spec, line, want string }{
		{"something to offer", `'-v[verbose]' '*:rest:'`, "cmd -", "0/-v:verbose"},
		// Spent, and not repeatable, so there is nothing left.
		{"everything spent", `'-v[verbose]' '*:rest:'`, "cmd -v -", "1/"},
		// **An argument specification carries an exclusion list too**, and
		// writing the argument is what spends it. `(-)` names every option,
		// which is the spelling the shipped `_git` writes twice.
		{"an argument excluded them", `'-v[verbose]' '(-)1:first:(a b)' '*:rest:'`, "cmd a -", "1/"},
		{"before it was written", `'-v[verbose]' '(-)1:first:(a b)' '*:rest:'`, "cmd -", "0/-v:verbose"},
		{"without the list", `'-v[verbose]' '1:first:(a b)' '*:rest:'`, "cmd a -", "0/-v:verbose"},
		{"a named option, not all", `'-v[verbose]' '(-v)1:first:(a b)' '*:rest:'`, "cmd a -", "1/"},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := `comparguments -i '' -s : ` + c.spec + `
				local -a n d od e
				comparguments -O n d od e
				say "$?/${n[*]}"`
			if got := reported(t, body, c.line); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestAHiddenSpecWearsItsExclamationFirst is the one-character reading that
// made `git checkout <TAB>` scribble a diagnostic over the line a person was
// typing.
//
// The three things that may stand in front of a specification have an order,
// and it is `!`, then the `(…)` exclusion list, then the `*`. Reading the
// list first refused `!(--no-guess)--guess`, which the shipped `_git` writes
// — and the refusal is *printed*, twice, before anything is offered.
//
// Measured on zsh 5.9.2, 2026-09-16: `!(-y)-x` is accepted, `(-y)!-x` is
// refused with `invalid argument`, and the exclusion list on the accepted one
// still applies once the hidden option is on the line.
func TestAHiddenSpecWearsItsExclamationFirst(t *testing.T) {
	for _, c := range []struct{ name, spec, line, want string }{
		{"accepted before the list", `'-y[why]' '!(-y)-x'`, "cmd -", "0/-y:why"},
		{"and it is still hidden", `'-y[why]' '!-x'`, "cmd -", "0/-y:why"},
		{"its list still applies", `'-y[why]' '!(-y)-x'`, "cmd -x -", "1/"},
		{"refused after the list", `'-y[why]' '(-y)!-x'`, "cmd -", "1/"},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := `comparguments -i '' -s : ` + c.spec + ` 2>/dev/null || { say "1/"; return }
				local -a n d od e
				comparguments -O n d od e
				say "$?/${n[*]}"`
			if got := reported(t, body, c.line); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// TestATagLoopBelongsToItsFunction is `comptags`' state being one per
// function nesting level rather than one per completion.
//
// The shipped completion system nests them: `_arguments` opens a loop over
// `argument-rest options`, the action it reaches calls `_alternative`, that
// opens a loop of its own, and when it is done the first loop has to still be
// there. With one slot the second install overwrote the first, so
// `git checkout <TAB>` ran the `modified-files` set and then found
// `tree-ishs` was not requested — the branch names were never reached.
//
// Measured on zsh 5.9.2, 2026-09-16 from inside a widget, with an outer loop
// over `a b` installed in the widget's own function and an inner one over
// `p q` installed in a function it calls:
//
//	after the inner function returns    -R a is 0, -R p is 1
//	inside a second function at the     -R p is 0 — the inner loop's own
//	  inner one's level                   level still holds it
//	inside one a level deeper           `no tags registered`
//
// which is the exact level and not the nearest one below it. The shipped
// `_tags` says so out loud in its own comment: a `--` first argument tells
// `comptags` to "use the preceding function nesting level".
func TestATagLoopBelongsToItsFunction(t *testing.T) {
	const helpers = `
		inner() { comptags -i :x:inner: p q; comptry p q; comptags -N }
		here() { comptags -R p 2>/dev/null; local x=$?; comptags -R a 2>/dev/null; say "$x$?" }
		deeper() { here }
	`
	for _, c := range []struct{ name, body, want string }{
		// The outer loop is back once the function that replaced it returns.
		{
			"the outer loop survives", `inner
			 comptags -R p; local x=$?; comptags -R a; say "$x$?"`,
			"10",
		},
		// And the inner one is still at its own level, which is where the
		// next function called from here stands.
		{"the inner loop is at its level", "inner; here", "01"},
		// A level nobody installed at has no loop at all, rather than the
		// nearest one below it.
		{"a level with no loop", "inner; deeper", "11"},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := helpers + `
				comptags -i :x:outer: a b; comptry a b; comptags -N
				` + c.body
			if got := reported(t, body, "cmd "); got != c.want {
				t.Errorf("%s answered %q, want %q", c.name, got, c.want)
			}
		})
	}
}
