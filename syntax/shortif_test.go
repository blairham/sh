// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The rule ShortForm stands for is about a header that has *ended*, and says
// nothing about looping — which is why it reaches `if`. The flag was called
// ShortLoop, and the name was the whole of why its coverage stopped at the
// loops (#827).
func TestAConditionThatEndedItselfTakesItsBody(t *testing.T) {
	short := Core()
	short.ShortForm = true
	short.DoubleBracket = true
	short.ArithCommand = true
	// `{ echo A }` with no terminator before the brace is a relaxation of
	// its own, and the rows below are written the way the shell with both
	// writes them.
	short.CloseBraceAlwaysReserved = true

	for _, src := range []string{
		"if [[ -n x ]] { echo A; }\n",
		"if (( 1 )) { echo A; }\n",
		"if [[ -n x ]] echo A\n",
		"if (( 1 )) echo A\n",
		"if [[ -z x ]] { echo A } else { echo B }\n",
		"if [[ -z x ]] { echo A } elif [[ -n y ]] { echo B } else { echo C }\n",
		// An and-or list finishes on its right side, and that is what has to
		// have ended.
		"if [[ -n x ]] && [[ -n y ]] { echo A }\n",
		// A `!` in front changes nothing, and the set is wider than the two
		// constructs the rule is usually stated with.
		"if ! [[ -z x ]] { echo A }\n",
		"if { true; } { echo A; }\n",
		"if ( true ) { echo A; }\n",
		"if case x in x) true;; esac { echo A; }\n",
		// A pipeline is judged on its last command. Found by mutation: the
		// rule had been "a pipeline never ends itself", which agrees with
		// `[[ … ]] | cat` for the wrong reason.
		"if true | [[ -n x ]] { echo A; }\n",
		"if true | { true; } { echo A; }\n",
	} {
		mustParse(t, src, short, "a condition that ended itself")
		mustFail(t, src, Core(), "the same text where the dialect has no short form")
	}

	// A word does not end a header, and a separator ends the whole command
	// rather than the header — the two rows the panel already agreed on.
	for _, src := range []string{
		"if true { echo A; }\n",
		"if true; { echo A; }\n",
		"if [[ -n x ]]; { echo A }\n",
		// A pipeline is judged on its *last* command, so this one does not
		// end itself — `cat` does not — while the row above does.
		"if [[ -n x ]] | cat { echo A }\n",
		// And a loop does not, which is measured rather than derived: it is
		// the one compound command with its own end that is refused here.
		"if for i in a; do true; done { echo A; }\n",
		"if while false; do :; done { echo A; }\n",
	} {
		mustFail(t, src, short, "a header that did not end itself")
	}

	// And the long form is untouched.
	for _, src := range []string{
		"if [[ -n x ]]; then echo A; fi\n",
		"if true; then echo A; else echo B; fi\n",
		"if true; then echo A; elif false; then echo B; fi\n",
	} {
		mustParse(t, src, short, "the long form under the flag")
		mustParse(t, src, Core(), "the long form without it")
	}
}

// Three constructs, three flags, because a shell could have any one without
// the others.
func TestTheOtherShortForms(t *testing.T) {
	d := Core()
	d.ShortForm = true
	d.Repeat = true
	d.Foreach = true
	d.AnonymousFunction = true

	for _, src := range []string{
		"repeat 3 { echo R; }\n",
		"repeat 3 echo R\n",
		"repeat 3; do echo R; done\n",
		"foreach f (a b); echo $f; end\n",
		"foreach f in a b\necho $f\nend\n",
		"() { echo x; }\n",
		"() { echo x; } p q\n",
		"() echo x\n",
		"function { echo x; }\n",
	} {
		mustParse(t, src, d, "a construct this dialect has")
	}

	// With the flag off the word is not a keyword. Some of these are then a
	// syntax error and some are an ordinary command — `repeat 3 echo R` is
	// a call of a command named `repeat` — so what is asserted is that the
	// construct is not what came back, rather than that nothing parsed.
	for _, tc := range []struct {
		src string
		off func(*Dialect)
	}{
		{"repeat 3 { echo R; }\n", func(x *Dialect) { x.Repeat = false }},
		{"repeat 3 echo R\n", func(x *Dialect) { x.Repeat = false }},
		{"foreach f (a b); echo $f; end\n", func(x *Dialect) { x.Foreach = false }},
		{"foreach f in a b\necho $f\nend\n", func(x *Dialect) { x.Foreach = false }},
		{"() { echo x; }\n", func(x *Dialect) { x.AnonymousFunction = false }},
		{"() echo x\n", func(x *Dialect) { x.AnonymousFunction = false }},
		{"function { echo x; }\n", func(x *Dialect) { x.AnonymousFunction = false }},
	} {
		without := d
		tc.off(&without)
		f, err := Parse(tc.src, without)
		if err != nil {
			continue
		}
		for _, st := range f.Stmts {
			pl, ok := st.Expr.(*Pipeline)
			if !ok {
				continue
			}
			switch pl.Cmds[0].(type) {
			case *RepeatClause, *AnonFunc:
				t.Errorf("%q parsed as the construct with its own flag off", tc.src)
			case *ForClause:
				t.Errorf("%q parsed as a loop with Foreach off", tc.src)
			}
		}
	}

	// `end` is reserved wherever a command may begin in a dialect with the
	// loop, and an ordinary word in one without it.
	mustFail(t, "end\n", d, "`end` alone")
	mustFail(t, "end() { :; }\n", d, "`end` as a function name")
	mustParse(t, "echo end\n", d, "`end` as an argument")
	mustParse(t, "end=5\n", d, "`end` as an assignment")
	mustParse(t, "end\n", Core(), "`end` where there is no such loop")
	// And the terminator belongs to the opening word.
	mustFail(t, "for f (a b); echo $f; end\n", d, "`end` closing a `for`")
}

// Printing has to produce source that means the same thing, and for these
// that means the long spelling wherever one exists.
func TestTheShortFormsPrintBack(t *testing.T) {
	d := Core()
	d.ShortForm = true
	d.Repeat = true
	d.Foreach = true
	d.AnonymousFunction = true
	d.DoubleBracket = true
	d.ArithCommand = true
	d.CloseBraceAlwaysReserved = true

	for _, src := range []string{
		"if [[ -n x ]] { echo A; }\n",
		"if (( 1 )) echo A\n",
		"if [[ -z x ]] { echo A } elif [[ -n y ]] { echo B } else { echo C }\n",
		"repeat 3 { echo R; }\n",
		"repeat 3 echo R\n",
		"foreach f (a b); echo $f; end\n",
		"() { echo x; } p q\n",
		"function { echo x; }\n",
	} {
		first, err := Parse(src, d)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		printed := Print(first)
		second, err := Parse(printed, d)
		if err != nil {
			t.Fatalf("%q printed as %q, which does not parse: %v", src, printed, err)
		}
		if again := Print(second); again != printed {
			t.Errorf("%q is not settled:\n  once:  %s\n  twice: %s", src, printed, again)
		}
	}
}

// Each arm of a short `if` chooses its own form, and the first one not
// written the short way puts the rest of the construct in the long one —
// where there is a `fi`, and it is required.
//
// #1372 filed the accepting half of this as "a short `else` arm with no body
// is accepted where the shell refuses it". Re-measuring found the rule one
// step further back: the `{` is what makes an arm short, an `else` that does
// not open one is a *long* arm, and an `else` with nothing after it is that
// arm arriving at the end of the input with the `fi` still owed. Written as
// "an empty short arm is an error" the first row below would have been
// refused too, and the shell runs it.
func TestAnArmNotWrittenShortPutsTheRestInTheLongForm(t *testing.T) {
	short := Core()
	short.ShortForm = true
	short.DoubleBracket = true
	short.ArithCommand = true
	short.CloseBraceAlwaysReserved = true

	for _, src := range []string{
		"if (( 0 )) { echo A } else echo B; fi\n",
		// The long arm's body is a list rather than the one command a short
		// body is, which is what says it is the long form and not a short
		// form that grew a terminator.
		"if (( 0 )) { echo A } else echo B; echo tail; fi\n",
		"if (( 0 )) { echo A } else\necho B\necho tail\nfi\n",
		// A compound command as the body: refused as a short arm, taken as a
		// long one, so the two readings are told apart by more than a `;`.
		"if (( 0 )) { echo A } else ( echo B ); fi\n",
		// The `elif` half, in both of the ways an arm stops being short: a
		// condition that never ended itself, and one that did with the body
		// on the next line.
		"if (( 0 )) { echo A } elif true; then echo C; fi\n",
		"if (( 0 )) { echo A } elif (( 1 ))\nthen echo C\nfi\n",
		// And the two spellings mix in one chain.
		"if (( 0 )) { echo A } elif (( 1 )) { echo C } else echo D; fi\n",
	} {
		mustParse(t, src, short, "a long arm after a short one")
	}

	for _, src := range []string{
		// #1372's own row: the `fi` never came.
		"if (( 1 )) { echo A } else\n",
		"if (( 0 )) { echo A } else echo B\n",
		"if (( 0 )) { echo A } else\necho B\n",
		"if (( 0 )) { echo A } else ( echo B )\n",
		"if (( 1 )) { echo A } elif (( 1 ))\n",
		// A brace group on the line after an `elif`'s condition is not a
		// short body: the newline ended the condition, so a `then` is owed
		// and a `{` is not one.
		"if (( 0 )) { echo A } elif (( 1 ))\n{ echo C }\n",
	} {
		mustFail(t, src, short, "a long arm without its `fi`")
	}

	// The short arm stays short, and the newlines in front of its brace do
	// not decide the form — an `else` has no condition for one to end. The
	// pair with the row below is what says so: no `fi` is owed here and one
	// written anyway is refused.
	mustParse(t, "if (( 0 )) { echo A } else { echo B }\n", short, "a short else")
	mustParse(t, "if (( 0 )) { echo A } else\n{ echo B }\n", short, "a newline before the brace")
	mustParse(t, "if (( 0 )) { echo A } else\n\n{ echo B }\n", short, "two of them")
	mustFail(t, "if (( 0 )) { echo A } else { echo B }\nfi\n", short, "a `fi` a short arm never owed")
}

// A short body that took the separator took the whole construct's, so there
// is no arm left for an `else` to attach to.
//
// The rule is the one shortIf's own doc states from the other side — the `;`
// ends the command — and it was stated there while the parser went on taking
// the `else`. Every row here is `parse error near `else“ on the shell with
// the construct, with or without a `fi` after it.
func TestASeparatedShortArmEndsTheWholeIf(t *testing.T) {
	short := Core()
	short.ShortForm = true
	short.DoubleBracket = true
	short.ArithCommand = true
	short.CloseBraceAlwaysReserved = true

	for _, src := range []string{
		"if (( 1 )) echo A; else echo B\n",
		"if (( 1 )) echo A; else echo B; fi\n",
		"if [[ -z x ]] echo A; else echo B; fi\n",
		"if (( 1 )) echo A; else { echo B }\n",
		"if (( 0 )) { echo A } elif (( 1 )) echo C; else { echo D }\n",
	} {
		mustFail(t, src, short, "an `else` after a separated short body")
	}

	// The controls, and they are what keep the rule from being "an `else`
	// after a short body is always refused": a brace body takes no separator,
	// so the arm after it attaches. A newline in place of the `;` reaches the
	// same refusal by a route that needed no code — it is still in hand, so
	// the `else` on the next line is not the token being looked at.
	mustParse(t, "if (( 1 )) { echo A } else { echo B }\n", short, "a brace body keeps the `else`")
	mustFail(t, "if (( 1 )) echo A\nelse echo B; fi\n", short, "a newline ends it too")
}

// A brace body is closed by its own `}` and takes no terminator with it,
// however deeply a short form written inside it took one of its own.
func TestABraceBodyTakesNoTerminatorFromWithin(t *testing.T) {
	short := Core()
	short.ShortForm = true
	short.CloseBraceAlwaysReserved = true

	mustFail(t, "for i (a b) { echo $i; } echo end\n", short,
		"a list carrying on after a brace body")
	// The nested row is the one that was wrong: the inner short loop's `;`
	// was left standing as the *outer* loop's terminator, so the tail ran
	// where the shell with the construct refuses it.
	mustFail(t, "for i (a b) { for j (c d) echo $j; } echo end\n", short,
		"an inner short body's separator standing as the outer loop's")

	// The controls: an unbraced short body does take the separator, so a list
	// may go on after it, and the nested shape is fine with nothing after it.
	mustParse(t, "for i (a b) echo $i; echo end\n", short, "a short body takes its separator")
	mustParse(t, "for i (a b) { for j (c d) echo $j; }\n", short, "the nesting itself")
}
