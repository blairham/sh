// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestANestedListsEmptyElements pins the two readings of the empty element a
// nested list hands back (#5412). Unquoted it is gone before the outer half
// reads anything, its subscript included. Quoted it stays, and it is a ghost:
// empty when printed or matched by an operator, one character long to a
// length, a search subscript and a pad. Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestANestedListsEmptyElements(t *testing.T) {
	const setup = "show() { print -rn -- \"$#:\"; for w; print -rn -- \"<$w>\"; print }\n" +
		"b=(xyz '' y); c=(''); c2=('' ''); c3=('' x); u=a::b; e=\n"
	for _, tc := range []struct{ src, want string }{
		// Unquoted: the empty elements are gone first.
		{`show ${${b[@]}[2]}`, "1:<y>\n"},
		{`show ${${b[@]}[-2]}`, "1:<xyz>\n"},
		{`show ${${c3[@]}[1]}`, "1:<x>\n"},
		{`show ${${(@s.:.)u}[2]}`, "1:<b>\n"},
		{`show ${${${b[@]}}[2]}`, "1:<y>\n"},
		{`show ${${b[@]}[(i)y]}`, "1:<2>\n"},
		{`show ${${b[@]}/#/p}`, "2:<pxyz><py>\n"},
		{`show x${^${b[@]}}y`, "2:<xxyzy><xyy>\n"},
		{`show ${${c2[@]}[1]-z}`, "1:<z>\n"},
		{`show ${${c2[@]}:-z}`, "1:<z>\n"},
		{`x=${${b[@]}[2]}; show $x`, "1:<y>\n"},
		{`show ${#${b[@]}[2]} ${#${b[@]#x}[1]} ${#${b[@]#x}[2]} ${#${c[@]}[1]}`, "4:<1><2><1><0>\n"},
		{`show ${(j:,:)${${b[@]}}}`, "1:<xyz,y>\n"},
		// Quoted: a ghost to a length.
		{`show "${#${b[@]}[2]}" "${#${c[@]}[1]}" "${#${(@)b}[2]}" "${#${(@s.:.)u}[2]}"`, "4:<1><1><1><1>\n"},
		{`show "${#${(@)${b[@]}}[2]}" "${#${(@)b[2,2]}[1]}" "${#${(@o)${b[@]}}[3]}"`, "3:<1><1><1>\n"},
		{`show "${(c)#${b[@]}}" "${(w)#${b[@]}}" "${(W)#${b[@]}}" "${(m)#${b[@]}[2]}"`, "4:<7><3><3><1>\n"},
		// The controls, which are 0: an element that is not there, a value
		// rather than a list, a level that picked one element or joined, and
		// an operator that rewrote the words.
		{`show "${#${b[@]}[5]}" "${#${b[@]}[(r)zz]}" "${#${b[2]}[1]}" "${#${(@)b[2]}[1]}"`, "4:<0><0><0><0>\n"},
		{`show "${#${${b[@]}[2]}}" "${(c)#${${b[@]}}}" "${(c)#${b[@]}#y}"`, "3:<0><6><5>\n"},
		// Quoted: a ghost to a search, which still names an empty element.
		{`show "${${b[@]}[(i)]}" "${${b[@]}[(I)]}" "${${b[@]}[(i)?]}" "${${b[@]}[(i)[^x]]}"`, "4:<4><0><2><2>\n"},
		{`show "${${b[@]}[(r)?]}" "${${b[@]}[(i)??]}"`, "2:<><4>\n"},
		// A pad: a nested *value* is not a ghost and pads to the full width.
		{`show "${(l:3:)${b[2]}}" "${(l:3:)${e}}" "${(@l:3:)${(@)b[2]}}" "${(@l:3:)${(@)b[2,2]}}"`, "4:<   ><   ><   ><  >\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestASortKeepsANestedSplitsBareFields pins that sorting the fields an `=`
// split a level down made keeps its empty ones unquoted, where the sort used
// to lose them. Measured 2026-10-02 on zsh 5.9.2 with `IFS=:` and `u=a::b:`.
func TestASortKeepsANestedSplitsBareFields(t *testing.T) {
	const setup = "show() { print -rn -- \"$#:\"; for w; print -rn -- \"<$w>\"; print }\nIFS=:; u=a::b:\n"
	for _, tc := range []struct{ src, want string }{
		{`show ${(@o)${=u}}`, "4:<a><b><><>\n"},
		{`show ${(@O)${=u}}`, "4:<><><b><a>\n"},
		{`show ${(@ou)${=u}}`, "3:<a><b><>\n"},
		{`show x${(@o)${=u}}y`, "4:<xa><b><><y>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
