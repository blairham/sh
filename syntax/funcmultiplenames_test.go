// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// `function a b { … }` — one body under several names. The grammar half;
// what the names are then bound to is dialect/zsh/funcmultiplenames_test.go.

// manyFuncNames is the core with the keyword, the hybrid parens and the name
// list, which is the combination one shell in the panel has.
func manyFuncNames() Dialect {
	d := Core()
	d.FunctionMultipleNames = true
	d.FunctionKeywordParens = true
	return d
}

// oneFuncName is the same dialect without the list, so every row below can
// say what the flag is doing rather than only that it parsed.
func oneFuncName() Dialect {
	d := manyFuncNames()
	d.FunctionMultipleNames = false
	return d
}

func decl(t *testing.T, src string, d Dialect) *FuncDecl {
	t.Helper()
	f, err := Parse(src, d)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	fn, ok := f.Stmts[0].Expr.(*Pipeline).Cmds[0].(*FuncDecl)
	if !ok {
		t.Fatalf("%s: not a function declaration", src)
	}
	return fn
}

// The names are read, all of them, and the flag is what reads them.
func TestTheKeywordTakesANameList(t *testing.T) {
	for _, tc := range []struct {
		src   string
		names []string
	}{
		{`function a b { :; }`, []string{"a", "b"}},
		{`function a b c { :; }`, []string{"a", "b", "c"}},
		{`function clipcopy clippaste { :; }`, []string{"clipcopy", "clippaste"}},
		// The hybrid parens close the list, so the last name keeps them.
		{`function a b() { :; }`, []string{"a", "b"}},
		{`function a b () { :; }`, []string{"a", "b"}},
		// A newline between the names and the body is a newline before a
		// body and not the end of the definition.
		{"function a b\n{ :; }", []string{"a", "b"}},
		// A backslash-newline between two names is a continuation, which is
		// how the construct is written in the wild.
		{"function man \\\n dman \\\n debman { :; }", []string{"man", "dman", "debman"}},
		// One name is still one name, with no list behind it.
		{`function a { :; }`, []string{"a"}},
	} {
		fn := decl(t, tc.src, manyFuncNames())
		got := append([]string{fn.Name}, nil...)
		for _, n := range fn.AlsoNamed {
			got = append(got, n.Name)
		}
		if len(got) != len(tc.names) {
			t.Errorf("%s: names %v, want %v", tc.src, got, tc.names)
			continue
		}
		for i := range got {
			if got[i] != tc.names[i] {
				t.Errorf("%s: names %v, want %v", tc.src, got, tc.names)
				break
			}
		}
	}
	// Without the flag the same text is refused, which is what says this is
	// additive grammar and is the failure #1680 reports: the second name is
	// read as the body, a simple command, and the brace group after it has
	// nothing to be part of — `parse error near `}`` on the definition's
	// closing brace, taking the whole file with it.
	one := oneFuncName()
	for _, src := range []string{
		`function a b { :; }`,
		`function a b c { :; }`,
		"function man \\\n dman \\\n debman { :; }",
	} {
		mustFail(t, src, one, "a name list without the flag")
	}
	// And the one-name definition still reads the same either way, which is
	// the control: nothing about the flag changes the grammar every dialect
	// already has.
	if got := decl(t, `function a { :; }`, one); got.Name != "a" || len(got.AlsoNamed) != 0 {
		t.Errorf("one name without the flag: %q %v", got.Name, got.AlsoNamed)
	}
}

// Names are taken greedily and stop where the body begins, which is the same
// rule ForMultipleNames follows and is measured rather than chosen: a
// reserved word after a name is a *name* in the shell that has this, so
// `function a while { … }` defines `while` and calling it afterwards runs the
// body rather than opening a loop.
func TestTheNameListIsGreedyAndStopsAtTheBody(t *testing.T) {
	d := manyFuncNames()
	for _, tc := range []struct {
		src   string
		names []string
	}{
		{`function a while { :; }`, []string{"a", "while"}},
		{`function a if for { :; }`, []string{"a", "if", "for"}},
		{`function a in { :; }`, []string{"a", "in"}},
	} {
		fn := decl(t, tc.src, d)
		if len(fn.AlsoNamed) != len(tc.names)-1 {
			t.Errorf("%s: %d extra names, want %d", tc.src, len(fn.AlsoNamed), len(tc.names)-1)
		}
	}
	// A stop word is not a name — it has nothing open to close, and the
	// refusal is what the shell that has the list gives too.
	for _, src := range []string{
		`function a } { :; }`,
		`function a b done { :; }`,
		`function a b then { :; }`,
	} {
		mustFail(t, src, d, "a stop word in the name list")
	}
}

// Every name is read by the rule the first one is read by, so the flags that
// say what a name may be apply to all of them rather than to the first.
func TestEveryNameInTheListIsReadTheSameWay(t *testing.T) {
	any := manyFuncNames()
	any.FunctionKeywordNameIsAnyWord = true
	fn := decl(t, `function a "b c" d { :; }`, any)
	if len(fn.AlsoNamed) != 2 || fn.AlsoNamed[0].Name != "b c" {
		t.Errorf(`function a "b c" d: extra names %v`, fn.AlsoNamed)
	}
	// Without that flag the quoted name is refused wherever it stands, which
	// is the control: the list did not widen what a name is.
	mustFail(t, `function a "b c" d { :; }`, manyFuncNames(), "a quoted name in the list")

	// An expansion in a name is kept as a word, in the second name as in the
	// first — flattening it to its literal text would name a different
	// function, which is the loss FuncDecl.NameWord exists to stop.
	exp := manyFuncNames()
	exp.FunctionNameExpands = true
	fn = decl(t, `function _p_${w} _q_${w} { :; }`, exp)
	if fn.NameWord == nil {
		t.Error("the first name lost its word")
	}
	if len(fn.AlsoNamed) != 1 || fn.AlsoNamed[0].Word == nil {
		t.Errorf("the second name lost its word: %v", fn.AlsoNamed)
	}
	// And without that flag it is refused in the list too, rather than being
	// flattened to `_q_w`.
	mustFail(t, `function a _q_${w} { :; }`, manyFuncNames(), "an expansion in a later name")

	// A bare pattern character is not a name here in any position, for the
	// reason keywordFuncName gives: the shell matches such a word against the
	// filesystem, and defining `a*b` at status 0 is a plausible wrong answer
	// where a refusal is a visible one.
	mustFail(t, `function a b*c { :; }`, any, "a bare pattern character in a later name")
}

// A printed definition has to be the same program, which is exactly where it
// would not be: printing the first name alone defines one function where the
// source defined three, at status 0, with the rest of the script calling
// names nobody defined.
func TestANameListPrintsBackWhole(t *testing.T) {
	d := manyFuncNames()
	d.FunctionKeywordNameIsAnyWord = true
	d.FunctionNameExpands = true
	for _, tc := range []struct{ src, want string }{
		{`function a b { :; }`, "function a b { :; }"},
		{`function a b c { :; }`, "function a b c { :; }"},
		{"function man \\\n dman \\\n debman { :; }", "function man dman debman { :; }"},
		{`function a "b c" d { :; }`, "function a 'b c' d { :; }"},
		{`function _p_${w} _q_${w} { :; }`, "function _p_$w _q_$w { :; }"},
		// The hybrid parens are not in the tree, so the keyword form comes
		// back — the same normalization a one-name hybrid already takes.
		{`function a b() { :; }`, "function a b { :; }"},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		got := Print(f)
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
			continue
		}
		if _, err := Parse(got, d); err != nil {
			t.Errorf("%s: printed form does not parse: %v", tc.src, err)
		}
	}
}

// The parenthesis spelling takes the same name list, and takes it from the
// *argument* loop rather than from a name test on one word — so any word list
// followed by `()` is a definition there, `echo hi () { … }` included.
func TestTheParenthesisSpellingTakesANameList(t *testing.T) {
	d := manyFuncNames()
	d.FunctionNameIsAnyWord = true
	for _, tc := range []struct {
		src   string
		names []string
	}{
		{`a b () { :; }`, []string{"a", "b"}},
		{`clipcopy clippaste() { :; }`, []string{"clipcopy", "clippaste"}},
		{`a b c () { :; }`, []string{"a", "b", "c"}},
		// A word that is a command name everywhere else is a name here,
		// which is what makes this the argument loop's reading and not a
		// widening of what a name may be.
		{`echo hi () { :; }`, []string{"echo", "hi"}},
		// A later name is read where an argument stands, so the `=` that
		// ends a *first* name is ordinary text in one behind it.
		{`a c=d () { :; }`, []string{"a", "c=d"}},
		// One name is still one name, with no list behind it.
		{`a () { :; }`, []string{"a"}},
	} {
		fn := decl(t, tc.src, d)
		got := append([]string{fn.Name}, nil...)
		for _, n := range fn.AlsoNamed {
			got = append(got, n.Name)
		}
		if len(got) != len(tc.names) {
			t.Errorf("%s: names %v, want %v", tc.src, got, tc.names)
			continue
		}
		for i := range got {
			if got[i] != tc.names[i] {
				t.Errorf("%s: names %v, want %v", tc.src, got, tc.names)
				break
			}
		}
	}
	// Without the flag the same text is a command followed by a parenthesis,
	// which every other dialect refuses there.
	for _, src := range []string{`a b () { :; }`, `clipcopy clippaste() { :; }`} {
		mustFail(t, src, oneFuncName(), "a name list before the parens without the flag")
	}
}

// Two shapes bound the reading, and both are measured rather than chosen: an
// assignment in front of the names ends it, and the body is not optional in
// this spelling the way it is after the keyword.
func TestTheParenthesisNameListStopsAtAnAssignmentAndWantsABody(t *testing.T) {
	d := manyFuncNames()
	d.FunctionNameIsAnyWord = true
	for _, src := range []string{
		`x=1 a b () { :; }`,
		`a b ()`,
	} {
		mustFail(t, src, d, "a bounded shape of the parenthesis name list")
	}
}

// Every name in the list is read by the rule the name in front of the
// parentheses is read by, which is what stops the list from being a wider
// reading of what a name may be.
func TestEveryNameBeforeTheParensIsReadTheSameWay(t *testing.T) {
	d := manyFuncNames()
	d.FunctionNameIsAnyWord = true
	// A quoted name is a name here because the dialect says any word is.
	fn := decl(t, `a "b c" () { :; }`, d)
	if len(fn.AlsoNamed) != 1 || fn.AlsoNamed[0].Name != "b c" {
		t.Errorf(`a "b c" (): extra names %v`, fn.AlsoNamed)
	}
	// A bare pattern character is matched against the filesystem in the shell
	// this models, so it names nothing there and must name nothing here —
	// in the list as in front of it.
	mustFail(t, `a*b c () { :; }`, d, "a bare pattern character in an earlier name")
	mustFail(t, `a b*c () { :; }`, d, "a bare pattern character in a later name")

	// An expansion in a name is kept as a word where the dialect expands one.
	exp := d
	exp.FunctionNameExpands = true
	fn = decl(t, `a _p_${w} () { :; }`, exp)
	if len(fn.AlsoNamed) != 1 || fn.AlsoNamed[0].Word == nil {
		t.Errorf(`a _p_${w} (): the later name lost its word: %v`, fn.AlsoNamed)
	}
	// And without that flag it is refused rather than flattened to `_p_w`,
	// which would be a perfectly good name for a different function.
	mustFail(t, `a _p_${w} () { :; }`, d, "an expansion in a later name")
}

// The parenthesis spelling prints back whole too, and it prints back in the
// spelling it was written in: the keyword is not in this tree, so nothing may
// put one there.
func TestTheParenthesisNameListPrintsBackWhole(t *testing.T) {
	d := manyFuncNames()
	d.FunctionNameIsAnyWord = true
	for _, tc := range []struct{ src, want string }{
		{`a b () { :; }`, "a b() { :; }"},
		{`clipcopy clippaste() { :; }`, "clipcopy clippaste() { :; }"},
		{`echo hi () { :; }`, "echo hi() { :; }"},
	} {
		f, err := Parse(tc.src, d)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		got := Print(f)
		if got != tc.want {
			t.Errorf("%s: printed %q, want %q", tc.src, got, tc.want)
			continue
		}
		back, err := Parse(got, d)
		if err != nil {
			t.Errorf("%s: printed form does not parse: %v", tc.src, err)
			continue
		}
		if why, ok := SameProgram(f, back); !ok {
			t.Errorf("%s: printed form is a different program: %s", tc.src, why)
		}
	}
}
