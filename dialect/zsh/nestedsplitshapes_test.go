// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedSplitKeepsItsEmptyFieldsAsZshDoes is #5312: what a nested
// expansion's split and its empty elements come to, outside a flag group and
// inside quotes. Every row measured on zsh 5.9.2 (/opt/homebrew/bin/zsh,
// -f, LC_ALL=C), 2026-10-01; `show` prints the count and then each word.
func TestANestedSplitKeepsItsEmptyFieldsAsZshDoes(t *testing.T) {
	const pre = `IFS=:; u=a::b:; b=(x "" y)
show() { printf '%s:' $#; for x in "$@"; do printf '<%s>' "$x"; done; print; }
`
	rows := []struct{ src, want string }{
		// An `=` a level down splits as an unquoted one does, quoted or not.
		{`show ${${=u}}`, "4:<a><><b><>"},
		{`show ${(@)${=u}}`, "4:<a><><b><>"},
		{`show "${(@)${=u}}"`, "4:<a><><b><>"},
		{`show "${(@q)${=u}}"`, "4:<a><><b><>"},
		{`show "${${=u}}"`, "1:<a::b:>"},
		{`show x${${=u}}y`, "4:<xa><><b><y>"},
		{`show "${(@)${=u}:-x}"`, "4:<a><><b><>"},
		{`show "${${=u}[2]}"`, "1:<>"},
		// …and an unquoted one drops its edges, so a quoted outer does not
		// bring them back.
		{`IFS=" "; v=" a  b "; show "${(@)${=v}}"`, "2:<a><b>"},
		// A quoted `s` a level down drops its empties as an unquoted one.
		{`show "${(@)${(s.:.)u}}"`, "2:<a><b>"},
		{`show "${(@q)${(s.:.)u}}"`, "2:<a><b>"},
		// …and with an `@` beside it the inner keeps them.
		{`show "${(@)${(@s.:.)u}}"`, "4:<a><><b><>"},
		{`show "${#${(@s.:.)u}}"`, "1:<4>"},
		// A count of a nested list counts only the words it would make.
		{`show ${#${b[@]}}`, "1:<2>"},
		{`show ${#${(s.:.)u}}`, "1:<2>"},
		{`show "${#${(@)${=u}}}"`, "1:<4>"},
		{`show "${#${=u}}"`, "1:<4>"},
		// Controls: an `=` on a list, and a nested list unquoted and quoted.
		{`show ${=b}`, "3:<x><><y>"},
		{`show ${(@q)${b[@]}}`, "2:<x><y>"},
		{`show "${(@q)${b[@]}}"`, "3:<x><><y>"},
		{`v="a*b:c"; show "${(@)${=v}}"`, "2:<a*b><c>"},
	}
	for _, row := range rows {
		out, _ := runZsh(t, t.TempDir(), pre+row.src+"\n")
		if out != row.want+"\n" {
			t.Errorf("%s\n got %q\nwant %q", row.src, out, row.want+"\n")
		}
	}
}

// TestANestedEmptyElementSortsAsZshDoes is the sort half of #5312: an empty
// element a nested parameter expansion hands back sorts as the byte 0xa1,
// and anything that rewrites the words on the way out puts it back first.
// Same shell, same day.
func TestANestedEmptyElementSortsAsZshDoes(t *testing.T) {
	const pre = `b=(x "" y)
show() { printf '%s:' $#; for x in "$@"; do printf '<%s>' "$x"; done; print; }
`
	rows := []struct{ src, want string }{
		{`show "${(@o)${b[@]}}"`, "3:<x><y><>"},
		{`show "${(@O)${b[@]}}"`, "3:<><y><x>"},
		{`show "${(@o)"${b[@]}"}"`, "3:<x><y><>"},
		{`b=("" x); show "${(@oi)${b[@]}}"`, "2:<x><>"},
		{`b=("" x); show "${(@on)${b[@]}}"`, "2:<x><>"},
		{`show "${(@oU)${b[@]}}"`, "3:<X><Y><>"},
		{`show "${(@o)${b[@]}-z}"`, "3:<x><y><>"},
		{`b=($'\xa2' "" $'\xa0'); show "${(@o)${b[@]}}"`, "3:<\xa0><><\xa2>"},
		// Not nested, or rewritten on the way out: the empty is first.
		{`show "${(@o)b}"`, "3:<><x><y>"},
		{`c=("${(@)b}"); show "${(@o)c}"`, "3:<><x><y>"},
		{`show "${(@o)${b[@]}#x}"`, "3:<><><y>"},
		{`show "${(@o)${b[@]}/x/w}"`, "3:<><w><y>"},
		{`show "${(@o)${b[@]}%q}"`, "3:<><x><y>"},
		{`show "${(@o)${b[@]}:#q}"`, "3:<><x><y>"},
		{`show "${(@os.:.)${:-x::y}}"`, "3:<><x><y>"},
		{`show "${(@of)"$(print -l x '' y)"}"`, "3:<><x><y>"},
	}
	for _, row := range rows {
		out, _ := runZsh(t, t.TempDir(), pre+row.src+"\n")
		if out != row.want+"\n" {
			t.Errorf("%s\n got %q\nwant %q", row.src, out, row.want+"\n")
		}
	}
}
