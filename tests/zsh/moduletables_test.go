// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A function defined in a `-c` string names the **shell** rather than a file,
// which is the one route with no file that still answers. Measured
// 2026-09-27, three routes over the same definition:
//
//	zsh -f -c 'f(){ : }; …'      zsh
//	zsh -f < a file of lines     (empty)
//	zsh -f script.zsh            script.zsh
//
// Its own test because the harness above has no route set, so nothing there
// can tell the first row from the second — and the difference is exactly
// what [interp.Runner.FunctionSourceFile] asks the route for. The standard
// input row is the control: a general fallback would have written the name
// in both.
func TestAFunctionsSourceFollowsTheRouteWhenThereIsNoFile(t *testing.T) {
	for _, tc := range []struct {
		name  string
		route interp.Route
		want  string
	}{
		{"a command string names the shell", interp.RouteCommandString, "f=[zsh]\n"},
		{"and standard input names nothing", interp.RouteStandardInput, "f=[]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "zmodload zsh/parameter\nf() { : }\nprint -r -- \"f=[${functions_source[f]}]\"\n"
			file, err := syntax.Parse(src, zsh.Dialect())
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			sem, dg, dl := zsh.Semantics(), zsh.Diagnostics(), zsh.Dialect()
			r := &interp.Runner{
				Semantics: &sem, Diagnostics: &dg,
				Stdout: &out, Stderr: &out,
				Name: "zsh", Dialect: &dl, Route: tc.route,
			}
			zsh.Apply(r)
			if _, err := r.Run(context.Background(), file); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Errorf("out = %q, want %q", out.String(), tc.want)
			}
		})
	}
}

// The seven of `zsh/parameter` that stopped refusing in #4909.
//
// Each read `parameter not implemented yet` at the expansion that asked, and
// each reports on something this shell already knew or can find out.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* for
// it — under `-f` from a script file with `env -i PATH=/usr/bin:/bin
// TERM=dumb` and a scratch `HOME`, `zsh/parameter` loaded on the first line.
func TestTheSevenTablesThatStoppedRefusing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The pattern characters, and the rows that say the list is a
			// **constant** rather than a reading of the option vector. The
			// obvious implementation computes it from the options, so all
			// four of the options that would change it are set here and the
			// answer must not move — which is what the reference does.
			"the pattern characters do not move with the options",
			`print -r -- "${(j:,:)patchars}"
			 setopt extendedglob; print -r -- "${(j:,:)patchars}"
			 setopt kshglob;      print -r -- "${(j:,:)patchars}"
			 setopt noglob;       print -r -- "${(j:,:)patchars}"`,
			strings.Repeat("|,~,(,?,*,[,<,^,#,?(,*(,+(,!(,\\!(,@(\n", 4),
		},
		{
			// And the disabled halves are empty, which is the control that
			// says the fifteen above are the live table rather than both of
			// them: `disable -p` and `disable -b` are not here, so nothing
			// has been taken out of it.
			"and the disabled halves are empty",
			`print -r -- "p=${#dis_patchars} b=${#dis_builtins}"`,
			"p=0 b=0\n",
		},
		{
			// The module roster, which is the set `zmodload` with no
			// operands already writes. The third line is what makes it a
			// view rather than a snapshot.
			"the module roster is what zmodload has loaded",
			`print -r -- "${(ko)modules} / ${modules[zsh/main]}"
			 zmodload zsh/datetime
			 print -r -- "${(ko)modules}"`,
			"zsh/main zsh/parameter / loaded\nzsh/datetime zsh/main zsh/parameter\n",
		},
		{
			// Where each function's body was read from, asked of a function
			// a *sourced file* defined so that the answer is a path rather
			// than the route's name. The second row is the one a fallback
			// would have got wrong: an autoload stub has no body yet, so
			// the key is there and the value is empty.
			"a function's source is the file its body was read from",
			`print 'sf() { : }' > lib.zsh
			 . ./lib.zsh
			 autoload -Uz af
			 print -r -- "sf=[${functions_source[sf]}]"
			 print -r -- "stub=[${functions_source[af]}] n=${#functions_source}"`,
			"sf=[./lib.zsh]\nstub=[] n=2\n",
		},
		{
			// The users looked up, which is none — and the **control is in
			// the same script**, because an empty table is also what a
			// parameter nobody wrote gives. `hash -d` fills the
			// neighboring table on the next line, so the reading is live
			// and it is this table that has nothing in it. Both rows hold
			// in the reference.
			//
			// The other half of that control — four successful `~user`
			// expansions leaving `${#userdirs}` at 0 there — is measured
			// against the binaries and written down in userdirs.go, because
			// this harness expands no tilde at all: it runs with no `HOME`,
			// so `~` is empty and `~root` stays literal, and a row here
			// would have been asserting the harness rather than the shell.
			"the users looked up are none, and the neighboring table fills",
			`print -r -- "userdirs=${#userdirs} nameddirs=${#nameddirs}"
			 hash -d zz=/tmp
			 print -r -- "userdirs=${#userdirs} nameddirs=${#nameddirs}"`,
			"userdirs=0 nameddirs=0\nuserdirs=0 nameddirs=1\n",
		},
		{
			// The history list cut into words, newest word first. `fc -R`
			// of a file of unquoted commands is the shape the two shells
			// agree on byte for byte — see historywords.go, where the three
			// cuts that shell has are measured and the one this shell takes
			// is argued.
			//
			// The *order* is the assertion and not the count: the newest
			// line first and, inside a line, the last word first, which is
			// the whole word sequence reversed. "The newest line's words in
			// order" agrees with that for a one-word line and parts from it
			// at the second, so the lines here carry three words each.
			"the history words are every word, most recently typed first",
			`print 'print one two' > h
			 print 'ls -l alpha beta' >> h
			 print 'echo tail' >> h
			 fc -R h
			 print -r -- "n=${#history} w=${(j:,:)historywords}"`,
			"n=2 w=beta,alpha,-l,ls,two,one,print\n",
		},
		{
			// And it is the list `$history` publishes rather than the raw
			// one the engine keeps, which holds the command now running:
			// reading that put the reading line's own words into the answer,
			// one entry ahead of `${#history}`.
			"and the words are the lines the history table holds",
			`print 'alpha beta' > h
			 print 'echo tail' >> h
			 fc -R h
			 print -r -- "h=${#history} w=${#historywords}"`,
			"h=1 w=2\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), "zmodload zsh/parameter\n"+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// `$usergroups` is the process's own group list, by name.
//
// Graded against `os.Getgroups` rather than against a list of group names,
// which is the only way to write this down: the answer is this machine's, and
// a fixture would be the machine the line was typed on. Sixteen entries here,
// and the reference answers the same sixteen — `staff -> 20`, `everyone ->
// 12`, `admin -> 80` and the rest — which is what says the source is right.
//
// The **count** is the assertion that cannot be satisfied by accident: a
// lookup that failed must still leave a row, under the number, because the
// length of this table and the length of `$GROUPS` are one fact.
func TestTheUserGroupsAreTheProcessesOwn(t *testing.T) {
	ids, err := os.Getgroups()
	if err != nil || len(ids) == 0 {
		t.Skip("no group list on this platform")
	}
	want := make([]string, 0, len(ids))
	for _, id := range ids {
		number := strconv.Itoa(id)
		name := number
		if g, err := user.LookupGroupId(number); err == nil && g.Name != "" {
			name = g.Name
		}
		want = append(want, name+"="+number)
	}
	sort.Strings(want)
	out, st := runZshPrelude(t, t.TempDir(), `zmodload zsh/parameter
print -r -- "n=${#usergroups}"
for k in ${(ko)usergroups}; do print -r -- "$k=$usergroups[$k]"; done`)
	wantOut := "n=" + strconv.Itoa(len(ids)) + "\n" + strings.Join(want, "\n") + "\n"
	if out != wantOut || st != 0 {
		t.Errorf("$usergroups = %q (status %d), want %q", out, st, wantOut)
	}
}

// And every one of the seven is frozen, hidden and described the way the
// module's own tables are.
//
// One loop over the seven names rather than a row each, because the pair is a
// property of being a module parameter — see hideModuleParameter — and the
// failure this guards is a name landing with the view and without the
// attributes, which is what every one of these had to be given by hand.
func TestTheSevenTablesCarryTheModuleAttributes(t *testing.T) {
	for _, tc := range []struct{ name, word string }{
		{"patchars", "array-readonly-hide-hideval-special"},
		{"historywords", "array-readonly-hide-hideval-special"},
		{"dis_builtins", "association-readonly-hide-hideval-special"},
		{"functions_source", "association-readonly-hide-hideval-special"},
		{"modules", "association-readonly-hide-hideval-special"},
		{"userdirs", "association-readonly-hide-hideval-special"},
		{"usergroups", "association-readonly-hide-hideval-special"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZshPrelude(t, t.TempDir(), `zmodload zsh/parameter
print -r -- "t=${(t)`+tc.name+`}"
`+tc.name+`=(a b)
print -r -- unreached`)
			want := "t=" + tc.word + "\nzsh:3: read-only variable: " + tc.name + "\n"
			if out != want || st != 1 {
				t.Errorf("$%s = %q (status %d), want %q", tc.name, out, st, want)
			}
		})
	}
}
