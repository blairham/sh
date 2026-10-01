// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// `~[name]` and `%~` ask the script's `zsh_directory_name` (#5150). Measured
// 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`); each row is what that shell wrote, with the
// scratch directory written as P.
func TestDynamicNamedDirectories(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "foo", "bar", "rod"), 0o700); err != nil {
		t.Fatal(err)
	}
	const fn = `zsh_directory_name() {
  emulate -L zsh
  setopt extendedglob
  local -a match mbegin mend
  if [[ $1 = d ]]; then
    if [[ $2 = (#b)(*bar)/rod ]]; then reply=(barmy ${#match[1]}); else return 1; fi
  else
    if [[ $2 = barmy ]]; then reply=($PWD/foo/bar); else return 1; fi
  fi
}
`
	for _, c := range []struct{ name, src, want string }{
		{"a name expands", "print -r -- ~[barmy]/anything", dir + "/foo/bar/anything\n"},
		{"and a prompt shortens to it", "cd foo/bar/rod; print -P %~", "~[barmy]/rod\n"},
		{"nonomatch leaves a miss as written", "setopt nonomatch; print -r -- ~[scuzzy]/rubbish", "~[scuzzy]/rubbish\n"},
		{"nomatch refuses it and ends the script", "print -r -- ~[scuzzy]/rubbish\nprint after", "zsh:11: no directory expansion: ~[scuzzy]\n"},
		{"only a word's leading tilde", "setopt nonomatch; print -r -- x~[barmy]", "x~[barmy]\n"},
		{"an empty reply is no answer", "zsh_directory_name() { reply=(); return 0 }; setopt nonomatch; print -r -- ~[E]", "~[E]\n"},
		{"nor is an empty string", "zsh_directory_name() { reply=(''); return 0 }; setopt nonomatch; print -r -- ~[E]/x", "~[E]/x\n"},
		{"the array's functions after the first", "unfunction zsh_directory_name; zsh_directory_name_functions=(f2); f2() { [[ $1 = n && $2 = G ]] && { reply=(/gee); return 0 }; return 1 }; print -r -- ~[G]", "/gee\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, dir, fn+c.src+"\n")
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// Which spelling `%~` draws is decided by how much of the path each claims.
func TestADynamicNameAgainstTheOtherNames(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "foo", "bar"), 0o700); err != nil {
		t.Fatal(err)
	}
	const fn = "d=$PWD\nzsh_directory_name() { [[ $1 = d && $2 = $d/foo* ]] && { reply=(F $((${#d}+4))); return 0 }; return 1 }\ncd foo/bar\n"
	for _, c := range []struct{ name, src, want string }{
		{"the home wins a tie", "HOME=$d/foo; print -P %~", "~/bar\n"},
		{"a named directory does not", "HOME=/nonexist; hash -d xx=$d/foo; print -P %~", "~[F]/bar\n"},
		{"a named directory claiming more wins", "HOME=/nonexist; hash -d yy=$d/foo/bar; print -P %~", "~yy\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, fn+c.src+"\n"); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
