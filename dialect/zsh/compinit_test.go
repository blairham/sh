// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// compinitFixture is a directory of completion files whose first lines are
// the shapes compinit reads: a `#compdef` with a service and a pattern, a
// `#compdef -P`, an `#autoload`, and a file that is neither.
func compinitFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Set rather than left to the umask, which another test in this package
	// may have moved: a group-writable fixture is one compaudit refuses.
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"_foo":    "#compdef foo bar=baz -p \"x*\"\nprint in _foo\n",
		"_helper": "#autoload\nprint helper\n",
		"_pat":    "#compdef -P \"*.ext\" qq\n",
		"_plain":  "nothing here\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestTheShippedCompinit drives share/sh/functions/compinit over the fixture
// against bytes recorded from zsh 5.9.2 on 2026-10-02 under `-f -c`, with the
// fixture ahead of zsh's own function directory: the tables the first lines
// fill, the services, the two pattern tables, what is autoloaded and what is
// not, and compdef's own forms (#5393). Nothing here reads zsh's function
// files; the probe calls them.
func TestTheShippedCompinit(t *testing.T) {
	fix := compinitFixture(t)
	out, _ := runShipped(t, `fpath=(`+fix+` $fpath); autoload -Uz compinit; compinit -D -u; echo st=$?
for k in foo bar qq x newcmd; do print -r -- "$k=${_comps[$k]-unset}"; done
print -r -- ${(t)_comps} ${(t)_services} "${_services[bar]-}"
print -r -- "pat=${_patcomps["x*"]-unset} post=${_postpatcomps["*.ext"]-unset} postqq=${_postpatcomps[qq]-unset}"
whence -w compdef compaudit compdump _foo _helper _plain
compdef _foo newcmd; print $_comps[newcmd]; compdef -n _other newcmd; print $_comps[newcmd]
compdef -d newcmd; print ${+_comps[newcmd]}; compdef _foo; echo st=$?
compdef _g g1=svc; print $_comps[g1] $_services[g1]`)
	want := "st=0\nfoo=_foo\nbar=_foo\nqq=unset\nx=unset\nnewcmd=unset\n" +
		"association-hideval association-hideval baz\npat=_foo post=_pat postqq=_pat\n" +
		"compdef: function\ncompaudit: function\ncompdump: function\n_foo: function\n_helper: function\n_plain: none\n" +
		"_foo\n_foo\n0\nst=0\n_g svc\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// TestTheShippedCompauditRefusesAnInsecureDirectory: a world-writable
// directory on $fpath is listed by compaudit — the heading on standard error,
// the directory on standard output, status 1 — and compinit with neither `-u`
// nor `-i` and no terminal aborts and takes itself away, where `-i` leaves the
// directory out. Recorded from zsh 5.9.2 on 2026-10-02 (#5393).
func TestTheShippedCompauditRefusesAnInsecureDirectory(t *testing.T) {
	fix := compinitFixture(t)
	insecure := filepath.Join(t.TempDir(), "insec")
	if err := os.Mkdir(insecure, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(insecure, "_insfoo"), []byte("#compdef insfoo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(insecure, 0o777); err != nil {
		t.Fatal(err)
	}
	out, _ := runShipped(t, `fpath=(`+fix+` $fpath); autoload -Uz compaudit compinit; compaudit; echo st=$?
fpath=(`+insecure+` $fpath); compaudit 2>/dev/null; echo st=$?; compaudit 2>&1 >/dev/null
compinit -D </dev/null 2>&1; echo st=$?; whence -w compinit
autoload -Uz compinit; compinit -D -i; echo st=$? ${_comps[insfoo]-unset} ${_comps[foo]-unset}`)
	out = strings.ReplaceAll(out, insecure, "INSEC")
	want := "st=0\nINSEC\nst=1\nThere are insecure directories:\n" +
		"not interactive and can't open terminal\n\ncompinit: initialization aborted\nst=1\ncompinit: none\n" +
		"st=0 unset _foo\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// TestTheShippedBashcompinit: `complete` records `_bash_complete` and its
// options as a word list, `complete -p` lists them back, and `compgen` lists
// what its options name, ignoring the word, between the prefix and the
// suffix. Recorded from zsh 5.9.2 on 2026-10-02 (#5393).
func TestTheShippedBashcompinit(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"fa", "fb"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "da"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runZsh(t, dir, `fpath=(`+shippedFunctionDir(t)+`); autoload -Uz compinit bashcompinit; compinit -D -u; bashcompinit
compgen -W "alpha beta" -- al; compgen -f -- f; compgen -d -- d; compgen -X nope; print st=$?
compgen -P pre -W "a b"; compgen -S suf -W "a b"
complete -F f1 c1; complete -o nospace -W "q r" c2; complete -p; complete -r c1; print $+_comps[c1]
print -r -- $_comps[c2]`)
	want := "alpha\nbeta\nda\nfa\nfb\nda\n\nst=0\nprea\npreb\nasuf\nbsuf\n" +
		"complete -F f1 c1\ncomplete -o nospace -W q\\ r c2\n0\n_bash_complete -o nospace -W q\\ r\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// TestTheShippedCompinitLeavesWhatMainCompleteReads is #6210: the names zsh's
// compinit creates beside its tables, with the types measured on zsh 5.9.2
// (`${(t)…}` after `compinit -D`), and a `_comp_setup` that, evaluated the
// way `_main_complete` evaluates it, gives a completion function the option
// state, `IFS` and traps measured inside one in zsh 5.9.2 — options flipped
// at the prompt come back, the ones zsh leaves alone stay, and nothing leaks
// out of the function that evaluated it.
func TestTheShippedCompinitLeavesWhatMainCompleteReads(t *testing.T) {
	out, _ := runShipped(t, `autoload -Uz compinit; compinit -D -u
for n in _lastcomp compprefuncs comppostfuncs _comp_options _comp_setup; do
	print -r -- "$n ${(tP)n}"
done
print -r -- "dump=${_comp_dumpfile:t} opts=${#_comp_options}"
setopt ksharrays shwordsplit nounset markdirs globsubst autocd globdots promptsubst
unsetopt nullglob extendedglob
trap 'print zerr' ZERR
probe() {
	eval "$_comp_setup"
	local o
	for o in ksharrays shwordsplit unset markdirs globsubst extendedglob nullglob rcexpandparam aliases autocd globdots promptsubst; do
		[[ -o $o ]] && print -rn -- "$o " || print -rn -- "no$o "
	done 2>/dev/null
	print
	print -r -- "IFS=${(q)IFS}"
	[[ $(trap) == *ZERR* ]] && print zerr || print nozerr
	(( $+functions[TRAPINT] && $+functions[TRAPQUIT] )) && print traps
}
probe
[[ -o ksharrays && -o globsubst && ! -o nullglob ]] && print restored
(( $+functions[TRAPINT] )) || print gone`)
	want := "_lastcomp association-hideval\ncompprefuncs array\ncomppostfuncs array\n" +
		"_comp_options array-hideval\n_comp_setup scalar-hideval\n" +
		"dump=.zcompdump opts=31\n" +
		"noksharrays noshwordsplit unset nomarkdirs noglobsubst extendedglob nullglob " +
		"rcexpandparam noaliases autocd globdots promptsubst \n" +
		"IFS=\\ $'\\t'$'\\r'$'\\n'$'\\0'\nnozerr\ntraps\nrestored\ngone\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// TestTabAfterTheShippedCompinitRunsAMainCompleteThatReadsItsState is the
// same through the editor: a `_main_complete` written here — not zsh's — that
// evaluates `$_comp_setup`, globs with nothing to match, records into
// `_lastcomp` and offers a match, reached by Tab after compinit put
// `complete-word` on it. With main's compinit the glob was `no matches
// found` and the assignment to `_lastcomp` a subscript error, the two
// failures zsh's own `_main_complete` hit on every Tab (#6210).
func TestTabAfterTheShippedCompinitRunsAMainCompleteThatReadsItsState(t *testing.T) {
	fix := compinitFixture(t)
	body := "#autoload\neval \"$_comp_setup\"\nlocal -a none; none=( " + fix + "/nomatch*_* )\n" +
		"_lastcomp[seen]=1\ncompadd -- \"matched${#none}\"\n"
	if err := os.WriteFile(filepath.Join(fix, "_main_complete"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := bindkeyRunner(t, "fpath=("+shippedFunctionDir(t)+" "+fix+")\nautoload -Uz compinit; compinit -D -u\n")
	line := "x mat"
	got := zsh.RunCompletion(r, t.Context(), "complete-word", repl.Completion{
		Line: line, Point: len(line), Start: 2, Word: "mat", Dir: r.Dir,
	})
	if len(got) != 1 || got[0].Word != "matched0" {
		t.Errorf("Tab offered %+v, want the one match matched0", got)
	}
	if v, _ := r.GetAssoc("_lastcomp"); v["seen"] != "1" {
		t.Errorf("_lastcomp = %v, want seen=1", v)
	}
}
