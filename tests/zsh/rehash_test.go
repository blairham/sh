// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// `rehash` and `unhash` are this shell's own names for clearing the command
// hash and for taking one entry out of a table. The *behavior* was here all
// along — `hash -r` empties the table — and the names were not, so a startup
// file that added a directory to `$path` and called `rehash` died on that
// line at 127, which reads to a script exactly like a typo (#3109).
//
// Measured 2026-09-18 on zsh 5.9.2 with `-f`, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and stdin `/dev/null`.
// Every row below is that shell's, byte for byte, including the refusals —
// which is why these are builtins rather than prelude functions wrapping
// `hash`: each has to refuse under **its own** name.
func TestRehashAndUnhashAreBuiltins(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"both are builtins", "whence -w rehash; whence -w unhash", "rehash: builtin\nunhash: builtin\n", 0},
		{"rehash clears the table", "zmodload zsh/parameter\ncommands[zqA]=/bin/echo\nrehash\nprint -r -- \"st=$? after=[$commands[zqA]]\"", "st=0 after=[]\n", 0},
		{"rehash takes -f and -d", "rehash -f; print $?; rehash -d; print $?", "0\n0\n", 0},
		{"and refuses every other letter", "rehash -x", "zsh:rehash:1: bad option: -x\n", 1},
		{"a word of its own is a count error", "rehash foo", "zsh:rehash:1: too many arguments\n", 1},
		{"unhash needs an operand", "unhash", "zsh:unhash:1: not enough arguments\n", 1},
		{
			"unhash takes one command out",
			"zmodload zsh/parameter\ncommands[zqA]=/bin/echo\nunhash zqA\nprint -r -- \"st=$? after=[$commands[zqA]]\"",
			"st=0 after=[]\n", 0,
		},
		{"a name no table holds", "unhash nosuchzz", "zsh:unhash:1: no such hash table element: nosuchzz\n", 1},
		{"-a is the alias table", "alias aa=1\nunhash -a aa\nprint \"$? [$(alias aa 2>&1)]\"", "0 []\n", 0},
		{"-s is the suffix aliases", "alias -s sfx=1\nunhash -s sfx\nprint \"$? [$(alias -s)]\"", "0 []\n", 0},
		{"-f is the functions", "f(){ :; }\nunhash -f f\nprint \"$? [$(whence -w f)]\"", "0 [f: none]\n", 0},
		{"-d is the named directories", "hash -d zd=/tmp\nunhash -d zd\nprint \"$? [$(hash -d)]\"", "0 []\n", 0},
		{
			"and every table answers a missing name the same way",
			"unhash -f nosuchzz; unhash -d nosuchzz; unhash -a nosuchzz",
			"zsh:unhash:1: no such hash table element: nosuchzz\n" +
				"zsh:unhash:1: no such hash table element: nosuchzz\n" +
				"zsh:unhash:1: no such hash table element: nosuchzz\n", 1,
		},
		{"-m over a pattern that matches nothing is 1", "unhash -m 'zq*'", "", 1},
		{
			"-m over one that matches is 0",
			"zmodload zsh/parameter\ncommands[zqA]=/bin/echo\nunhash -m 'zq*'\nprint \"$? [$commands[zqA]]\"",
			"0 []\n", 0,
		},
		{"and unhash refuses the letters it has not got", "unhash -z zz", "zsh:unhash:1: bad option: -z\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want || st != c.status {
				t.Errorf("%s =\n%q at %d\nwant\n%q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
