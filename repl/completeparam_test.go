// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
)

// Completing a word whose directory portion is a parameter — #1574.
//
// Every want below is what the panel did, driven through a pseudo-terminal on
// 2026-09-08 in a scratch home holding `documents/target-file.txt`, reading the
// line the editor left rather than the output of the command: `echo` expands a
// tilde itself, so grading the *output* cannot tell a completion from an
// expansion.
//
//	typed                        bash 5.3.15        bash 3.2.57   zsh 5.9.2
//	echo $HOME/docum<TAB>        $HOME/documents/   /…/documents/ $HOME/documents/
//	echo ${HOME}/docum<TAB>      ${HOME}/documents/ —             —
//	echo "$HOME"/docum<TAB>      $HOME/documents/   —             —
//	echo $(echo documents)/tar   bell               —             —
//	echo docum*/tar<TAB>         bell               —             —
//
// Three of the four columns that complete answer a word whose directory
// portion is a parameter, and they differ only in what they put *back* in the
// line. This shell offered nothing at all, in every configuration, because it
// looked for a directory really called `$HOME`.

// paramFixture is the scratch home those rows were measured in, and the
// completer that reads it. The seam is the real one for the reason
// spellCompleter's is: the question is whether the expansion reaches the
// completer at all.
func paramFixture(t *testing.T) runnerCompleter {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "documents", "target-file.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &interp.Runner{Dir: home}
	r.SetVar("HOME", home)
	r.SetVar("SUB", "documents")
	return runnerCompleter{r: r}
}

// The row the issue is about, in the three spellings bash was measured in.
//
// What comes back is the text as *typed* with the last component filled in —
// bash's `direxpand`-off state and zsh's only state — so the parameter is
// still a parameter in the line afterwards. That is the half this must not get
// wrong: a completion that wrote the expansion into the line would have
// answered the same question with a different, and measurably wrong, answer.
func TestADirectoryPortionThatIsAParameterCompletes(t *testing.T) {
	for _, tc := range []struct{ name, typed, want string }{
		{"bare", "$HOME/docum", "$HOME/documents/"},
		{"braced", "${HOME}/docum", "${HOME}/documents/"},
		// bash writes `$HOME/documents/` here, dropping the quotes it was
		// given; this shell keeps the text as typed, quotes and all, which is
		// the same rule that keeps `~/Deve` as `~/Developer/` rather than
		// spelling somebody's home directory out. Requoting the line is a
		// separate behavior from expanding the directory, and #1574 is the
		// second one — the row that matters is that something is offered at
		// all, where before there was nothing.
		{"quoted", `"$HOME"/docum`, `"$HOME"/documents/`},
		{"a name that is not HOME", "$SUB/targ", "$SUB/target-file.txt"},
		{"a parameter that is the whole path", "$HOME/", "$HOME/documents/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := completeWord(paramFixture(t), tc.typed)
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("completing %q offered %q, want [%q]", tc.typed, got, tc.want)
			}
		})
	}
}

// The boundary, and the reason this is not simply interp.Runner.Expand: a word
// whose directory portion would have to *run* something is refused, so Tab
// offers nothing rather than a command's output. bash rings the bell here and
// nothing else, which is the same answer from the person's side.
//
// The nested rows are the ones a scan of the text would miss and the ones that
// would actually hurt: the substitution is inside a parameter's default, its
// subscript, and an inner expansion, and in each the outer span is a parameter
// expansion like any other.
func TestADirectoryPortionThatWouldRunSomethingIsRefused(t *testing.T) {
	for _, typed := range []string{
		"$(echo documents)/tar",
		"`echo documents`/tar",
		"${UNSET:-$(echo documents)}/tar",
		"${UNSET:=`echo documents`}/tar",
		"$((1+1))/tar",
		"<(echo documents)/tar",
	} {
		t.Run(typed, func(t *testing.T) {
			if got := completeWord(paramFixture(t), typed); len(got) != 0 {
				t.Errorf("completing %q offered %q, want nothing", typed, got)
			}
		})
	}
}

// A dollar somebody marked as meaning itself is not a parameter, so the
// directory is read as the characters that were typed. Both marks, because
// dequote takes both off and the expansion would otherwise see the same text
// either way.
//
// There is no `$HOME` directory in the fixture, so an expansion here would
// complete and a literal reading offers nothing — which is what makes these
// rows tell the two apart rather than merely agree with each other.
func TestAnEscapedOrQuotedDollarIsNotAParameter(t *testing.T) {
	for _, typed := range []string{`\$HOME/docum`, `'$HOME'/docum`} {
		t.Run(typed, func(t *testing.T) {
			if got := completeWord(paramFixture(t), typed); len(got) != 0 {
				t.Errorf("completing %q offered %q, want nothing", typed, got)
			}
		})
	}
}

// A glob in the directory portion is not expanded either — bash rings the bell
// — so the pattern is read as a name and there is no directory by that name.
func TestAGlobInTheDirectoryPortionIsNotExpanded(t *testing.T) {
	if got := completeWord(paramFixture(t), "docum*/targ"); len(got) != 0 {
		t.Errorf("offered %q, want nothing", got)
	}
}

// The control, and the reason the rows above are about the parameter rather
// than about the fixture: the same word with the directory written out
// completes throughout.
func TestTheSameWordWithoutAParameterStillCompletes(t *testing.T) {
	got := completeWord(paramFixture(t), "documents/targ")
	if len(got) != 1 || got[0] != "documents/target-file.txt" {
		t.Errorf("offered %q, want [documents/target-file.txt]", got)
	}
}
