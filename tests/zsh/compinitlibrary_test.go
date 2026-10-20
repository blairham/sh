// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped compinit over a library laid out the way an installed zsh's
// is — files reached through links, a `_main_complete`, an `#autoload +X`
// file — answers as zsh's compinit does over one. Measured 2026-10-05 on
// zsh 5.9.2 against /opt/homebrew/share/zsh, whose files are links into the
// Cellar:
//
//   - a link to a `#compdef` file registers (#6183) — `_comps[git]` is
//     `_git`, where `_*(N.)` skipped every link and registered nothing;
//   - each name is autoloaded from its own path, so `whence -v` names the
//     file (#6155) — `_git is an autoload shell function from …/_git`;
//   - with a `_main_complete` found, the eight completion widgets are put on
//     it (#6184) — `zle -lL` lists them — and with none found, nothing is;
//   - an `#autoload +X` file is loaded on the spot by its path (#6186).
func TestTheShippedCompinitOverAnInstalledLibrary(t *testing.T) {
	lib := t.TempDir()
	if err := os.Chmod(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	cellar := t.TempDir()
	if err := os.Chmod(cellar, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"_linked":        "#compdef linkcmd\n",
		"_main_complete": "#autoload\n",
		"_loadnow":       "#autoload +X\n_loadnow() { print loaded }\n",
	} {
		if err := os.WriteFile(filepath.Join(cellar, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(cellar, name), filepath.Join(lib, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(lib, "_plainfile"), []byte("#compdef plaincmd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := runShipped(t, `fpath=($fpath `+lib+`); autoload -Uz compinit; compinit -D -u; echo st=$?
print -r -- "linkcmd=${_comps[linkcmd]-unset} plaincmd=${_comps[plaincmd]-unset}"
whence -v _linked _main_complete
whence -w _loadnow
local -a w; w=(${(f)"$(zle -lL)"}); print ${#${(M)w:#* _main_complete}}`)
	out = strings.ReplaceAll(out, lib, "LIB")
	want := "st=0\nlinkcmd=_linked plaincmd=_plainfile\n" +
		"_linked is an autoload shell function from LIB/_linked\n" +
		"_main_complete is an autoload shell function from LIB/_main_complete\n" +
		"_loadnow: function\n8\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// And with no `_main_complete` anywhere on the search, Tab keeps this shell's
// own completion: no widget is put on a function that is not there.
func TestTheShippedCompinitBindsNothingWithoutMainComplete(t *testing.T) {
	out, _ := runShipped(t, `autoload -Uz compinit; compinit -D -u; local -a w; w=(${(f)"$(zle -lL)"}); print ${#${(M)w:#* _main_complete}}`)
	if out != "0\n" {
		t.Errorf("widgets on _main_complete = %q, want none", out)
	}
}

// TestTheShippedCompinitMovesTabWhenTheCompleterExpands is #6216: with
// `_expand` among the words of the `completer` style at `:completion:` and
// Tab on `expand-or-complete`, compinit puts Tab on `complete-word`, as
// zshcompsys(1) says. Every row recorded from zsh 5.9.2 under `-f -c`,
// 2026-10-05, `bindkey '^I'` after `compinit -D`.
func TestTheShippedCompinitMovesTabWhenTheCompleterExpands(t *testing.T) {
	lib := t.TempDir()
	if err := os.Chmod(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "_main_complete"), []byte("#autoload\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ setup, tab string }{
		{`zstyle ':completion:*' completer _expand _complete`, "complete-word"},
		{`zstyle ':completion:' completer _complete _expand`, "complete-word"},
		{`zstyle '*' completer _expand`, "complete-word"},
		{`zstyle ':completion:*:*:*' completer _expand _complete`, "expand-or-complete"},
		{`zstyle ':completion:::::' completer _expand _complete`, "expand-or-complete"},
		{`zstyle ':completion:*' completer _expand_alias _complete`, "expand-or-complete"},
		{`zstyle ':completion:*' completer _expand:foo _complete`, "expand-or-complete"},
		{`zstyle ':completion:*' completer _expand; bindkey '^I' menu-complete`, "menu-complete"},
		{`:`, "expand-or-complete"},
	} {
		t.Run(c.setup, func(t *testing.T) {
			out, _ := runShipped(t, `fpath=($fpath `+lib+`); `+c.setup+`
autoload -Uz compinit; compinit -D -u; bindkey '^I'`)
			if want := "\"^I\" " + c.tab + "\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
	// The main keymap only: under `bindkey -v` that is `viins`.
	out, _ := runShipped(t, `fpath=($fpath `+lib+`); zstyle ':completion:*' completer _expand; bindkey -v
autoload -Uz compinit; compinit -D -u; bindkey -M viins '^I'; bindkey -M emacs '^I'`)
	if want := "\"^I\" complete-word\n\"^I\" expand-or-complete\n"; out != want {
		t.Errorf("under bindkey -v: got %q, want %q", out, want)
	}
}
