// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What a match specification lets match, row for row against zsh.
//
// Every row was measured on zsh 5.9.2, 2026-10-05, from a widget running
// `PREFIX=word; compadd -O out -M spec -- candidates` and printing `out`,
// so that what is graded is the builtin's matching alone; the same widget
// through this shell gives the same 47 lines. `-M` was read and dropped until
// #6152, so every row here that is not a plain prefix match failed.
func TestAMatchSpecificationWidensWhatMatches(t *testing.T) {
	for _, c := range []struct{ spec, word, candidates, want string }{
		{"m:{a-zA-Z}={A-Za-z}", "RE", "README.md repl rpc.go Rx readme", "README.md repl readme"},
		{"m:{a-zA-Z}={A-Za-z}", "re", "README.md repl rpc.go Rx readme", "README.md repl readme"},
		{"m:{a-z}={A-Z}", "re", "README.md repl rpc.go Rx readme", "README.md repl readme"},
		{"m:{a-z}={A-Z}", "RE", "README.md repl rpc.go Rx readme", "README.md"},
		{"m:{[:lower:]}={[:upper:]}", "re", "README.md repl Rx", "README.md repl"},
		{"m:{[:lower:][:upper:]}={[:upper:][:lower:]}", "rE", "README.md repl Rx rE", "README.md repl rE"},
		{"r:|[._-]=* r:|=*", "f.b", "foo.bar.baz foo.baz fb f.b fxb foo_bar", "foo.bar.baz foo.baz f.b"},
		{"r:|[._-]=*", "f.b", "foo.bar.baz foo.baz fb f.b fxb", "foo.bar.baz foo.baz f.b"},
		{"r:|.=*", "..u", "comp.sources.unix comp.unix c.s.u", "comp.sources.unix c.s.u"},
		{"r:|.=*", ".u", "comp.sources.unix comp.unix .unix", "comp.unix .unix"},
		{"l:|=* r:|=*", "pl", "repl rpc.go apple plum xyz", "repl apple plum"},
		{"l:|=*", "pl", "repl rpc.go apple plum xyz", "repl apple plum"},
		{"r:|=*", "pl", "repl rpc.go apple plum xyz", "plum"},
		{"m:-=_", "a-b", "a_b a-b a_bc axb", "a_b a-b a_bc"},
		{"M:_=", "f_o", "foo fo f_oo", "foo fo f_oo"},
		{"b:-=+", "--x", "++x +-x --x -+x x", "+-x --x"},
		{"e:-=+", "x--", "x++ x+- x-- x", "x--"},
		{"r:?||[[:upper:]]=*", "fB", "fooBar fooHooBar fB Bar", "fooBar fB"},
		{"l:.||[[:alpha:]]=by", "pass.n", "pass.byname pass.name", "pass.byname pass.name"},
		{"r:|[.]=** r:|=*", "a.c", "ab.cd.ef abc a.b.c", "ab.cd.ef a.b.c"},
		{"r:|[.]=*", "a.c", "ab.cd.ef abc a.b.c", "ab.cd.ef"},
		{"x: m:{a-z}={A-Z}", "re", "README repl", "repl"},
		{"m:{a-z}={A-Z} x:", "re", "README repl", "README repl"},
		{"l:|=* r:|=*", "E", "README repl", "README"},
		{"m:?=x", "ab", "xb ab ax xx", "xb ab ax xx"},
		{"m:a=?", "ab", "ab xb bb", "ab xb bb"},
		{"m:[ab]=[xy]", "a", "a x y b", "a x y"},
		{"m:{ab}={xy}", "ab", "xy ay xb ab yx", "xy ay xb ab"},
		{"m:{abc}={xy}", "c", "c x y z", "c"},
		{"b:-=+", "-x", "+x -x ++x", "+x -x"},
		{"b:-=+", "--x", "++x +-x -+x --x", "+-x --x"},
		{"b:[-+]=[-+]", "--x", "++x +-x -+x --x", "+-x --x"},
		{"B:-=+", "--x", "++x +-x -+x --x", "+-x --x"},
		{"e:-=+", "x-", "x+ x- x+y", "x-"},
		{"e:-=+", "x--", "x++ x+- x-+ x--", "x--"},
		{"m:{a-z}={A-Z} r:|[._-]=* r:|=*", "r.m", "README.md readme.md Rx.Mx r.m rpc.go", "README.md readme.md Rx.Mx r.m"},
		{"r:|[._-]=* r:|=*", "f-b", "foo-bar foo_bar fb f-b foo-x-bar", "foo-bar f-b"},
		{"l:|=* r:|=*", "ME", "README.md readme NAME me", "README.md NAME"},
		{"r:[^[:upper:]]||[[:upper:]]=**", "fB", "fooBar fooHooBar fooBarBaz", "fooBar fooHooBar fooBarBaz"},
		{"l:x|=*", "ab", "ab xab", "ab"},
		{"r:|=*", "", "a b", "a b"},
		{"r:a|b=*", "ab", "ab axxb aXb", "ab axxb aXb"},
		{"r:a|b=**", "ab", "ab axxb abxb axbb", "ab axxb abxb axbb"},
		{"r:|b=*", "ab", "ab axb axbxb abxb", "ab axb axbxb abxb"},
		{"r:|b=**", "ab", "ab axb axbxb abxb", "ab axb axbxb abxb"},
		{"l:a|b=*", "ab", "ab axb axxb aab", "ab axb axxb aab"},
		{"l:a|=*", "ab", "ab axb xab", "ab axb"},
	} {
		t.Run(c.spec+"/"+c.word, func(t *testing.T) {
			body := "local -a out; PREFIX=" + shellQuote(c.word) +
				"; compadd -O out -M " + shellQuote(c.spec) + " -- " + c.candidates +
				"; say \"${out[*]}\""
			if got := reported(t, body, "x "); got != c.want {
				t.Errorf("-M %q with %q typed matched %q, want %q", c.spec, c.word, got, c.want)
			}
		})
	}
}

// What is put on the line for a lone match, measured through Tab on zsh
// 5.9.2, 2026-10-05: a lower-case matcher inserts the candidate, and an
// upper-case one writes the word's own characters into it where they matched.
func TestAnUpperCaseMatcherWritesTheWordIn(t *testing.T) {
	for _, c := range []struct{ spec, candidate, word, want string }{
		{"M:_=", "foo", "f_o", "f_oo"},
		{"m:_=", "foo", "f_o", "foo"},
		{"L:|-=", "foo", "-f", "-foo"},
		{"l:|-=", "foo", "-f", "foo"},
		{"b:-=+", "+x", "-x", "+x"},
		{"m:{a-z}={A-Z}", "README.md", "rea", "README.md"},
		{"M:{a-z}={A-Z}", "README", "rea", "reaDME"},
		{"L:|=* r:|=*", "apple", "pl", "ple"},
	} {
		t.Run(c.spec, func(t *testing.T) {
			got := completionFor(t, widgetOf("compadd -M "+shellQuote(c.spec)+" -- "+c.candidate), "x "+c.word)
			if len(got) != 1 || got[0] != c.want {
				t.Errorf("-M %q with %q typed offered %q, want [%s]", c.spec, c.word, got, c.want)
			}
		})
	}
}
