// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `$widgets` and `$keymaps`, the `zsh/zleparameter` module.
//
// The spellings were measured on zsh 5.9.2 (Homebrew, aarch64) with a scratch
// HOME and no startup files. The *roster* is this shell's own and not zsh's
// 386 widgets, for the reason zle.go gives about `zle -la`: a name in the
// answer is a claim that pressing a key bound to it does something.

// zleParam runs src and returns everything it wrote.
func zleParam(t *testing.T, src string) string {
	t.Helper()
	out, _, errs := runZshSplit(t, t.TempDir(), src)
	return out + errs
}

// TestAWidgetSaysWhichKindItIs is the parameter's whole content: a name maps
// to what the widget *is*, in one of two spellings this shell has.
//
// `user:` carries the **function's** name and not the widget's, which is the
// row that separates the two: `zle -N other f` is `user:f`, and only a shell
// that recorded the function can say so. A shell that wrote `user:` plus the
// widget's own name would pass the first row and fail the second.
//
// The third spelling is `zle -C`'s, and it is three parts: the completer this
// shell will run and then the function it calls, in that order — measured, so
// a shell that wrote them the other way round is caught by the same row.
func TestAWidgetSaysWhichKindItIs(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"one the editor performs itself",
			`print -r -- "[$widgets[backward-char]]"`,
			"[builtin]\n",
		},
		{
			"one defined over a function of its own name",
			"f(){ }\nzle -N mywidget\n" + `print -r -- "[$widgets[mywidget]]"`,
			"[user:mywidget]\n",
		},
		{
			"one defined over a function of another name",
			"f(){ }\nzle -N other f\n" + `print -r -- "[$widgets[other]]"`,
			"[user:f]\n",
		},
		{
			"a definition shadows the action it takes the name of",
			"f(){ }\nzle -N backward-char f\n" + `print -r -- "[$widgets[backward-char]]"`,
			"[user:f]\n",
		},
		{
			"one `zle -C` defined names its completer and its function",
			"f(){ }\nzle -C c .complete-word f\n" + `print -r -- "[$widgets[c]]"`,
			"[completion:.complete-word:f]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := zleParam(t, c.src); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// TestANameThatIsNotAWidgetReadsEmpty, and this parameter is the one produced
// table in the dialect that does *not* refuse.
//
// The difference from `$terminfo` and `$langinfo` is that the table is
// complete: this shell knows every widget it has, so a name that is not in it
// is a name that is not a widget. Nothing is being hidden by the empty string,
// and `${+widgets[nosuch]}` of 0 is a true answer rather than a guess.
func TestANameThatIsNotAWidgetReadsEmpty(t *testing.T) {
	got := zleParam(t, `print -r -- "${+widgets[nosuch]} [${widgets[nosuch]}]"`)
	if want := "0 []\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestWidgetsIsAViewAndNotASnapshot: `zle -N` is what a plugin runs, so a
// table filled in at startup would report the widgets that existed before the
// plugins loaded.
//
// Read, define, read again in one shell — the only shape that tells a view
// from a snapshot taken early.
func TestWidgetsIsAViewAndNotASnapshot(t *testing.T) {
	got := zleParam(t, `print -r -- "[${widgets[later]}]"`+"\n"+
		"f(){ }\nzle -N later f\n"+
		`print -r -- "[${widgets[later]}]"`)
	if want := "[]\n[user:f]\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestKeymapsIsTheSameListBindkeyPrints: the array and `bindkey -l` read one
// table, so the two cannot drift apart.
//
// The order is this shell's rather than zsh's, deliberately: zsh's `$keymaps`
// comes out in its hash table's order — `visual viopp command .safe vicmd main
// isearch viins emacs`, neither sorted nor the definition order — while
// `bindkey -l` in the same shell prints the same nine sorted. A list with no
// stated order is not one to imitate.
func TestKeymapsIsTheSameListBindkeyPrints(t *testing.T) {
	got := zleParam(t, `print -r -- "$keymaps"`+"\nbindkey -l\n")
	names := ".safe command emacs isearch main vicmd viins viopp visual"
	want := names + "\n" + strings.ReplaceAll(names, " ", "\n") + "\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestBothAreReadonlyAndHidden, and the two are different kinds: `typeset -p`
// writes `typeset -Ar widgets` for the association and `typeset -ar keymaps`
// for the array, which is measured and is what says `$keymaps` is a list of
// names rather than a table.
func TestBothAreReadonlyAndHidden(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a widget assignment", "widgets[x]=y", "zsh:1: read-only variable: widgets\n"},
		{"unsetting the widgets", "unset widgets", "zsh:1: read-only variable: widgets\n"},
		{"a keymap assignment", "keymaps[1]=y", "zsh:1: read-only variable: keymaps\n"},
		{"the widgets listing", "typeset -p widgets", "typeset -Ar widgets\n"},
		{"the keymaps listing", "typeset -p keymaps", "typeset -ar keymaps\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := zleParam(t, c.src); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

// TestTheZleParameterModuleLoads is the line whose failure took a plugin down.
//
// #1618: two syntax highlighters open with `zmodload zsh/zleparameter
// 2>/dev/null || { print failed loading …; return 1 }`, so the refusal was
// followed by `failed binding ZLE widgets, exiting` and no highlighting at
// all. The listing is asserted with it, because a module that loads while
// naming nothing would pass a bare status check.
func TestTheZleParameterModuleLoads(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/zleparameter && zmodload -lF zsh/zleparameter\n")
	if want := "+p:keymaps\n+p:widgets\n"; out != want || st != 0 {
		t.Errorf("output = %q status %d, want %q and 0", out, st, want)
	}
}
