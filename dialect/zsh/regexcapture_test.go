// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What `=~` captured is readable here, and under the **same** parameters a
// reporting pattern fills rather than under a record of this operator's own.
//
// It is a dialect's answer twice over. That these parameters are written at
// all is the switch interp/regexmatch.go exposes and zsh.Apply throws; that
// the numbers count from one is the array base; and that `$match` holds the
// groups without the whole match is the shape, which the other shell with
// `=~` does not share.
//
// Measured against zsh 5.9.2 on 2026-09-11 under "env -i HOME=... zsh -f",
// byte for byte. Three of the lines are the ones an implementation gets wrong
// by being reasonable: a **failing** match leaves every one of them holding
// what the match before it put there, a pattern with **no groups** leaves
// "$match" alone rather than emptying it, and a group that did not
// participate is an empty element at -1 so the group after it keeps its
// number. The last line is the scope: these are ordinary parameters, so a
// match inside a function is still readable after it returns.
//
// Without this, "regexp-replace" cannot be written: a global search and
// replace needs each match's *extent* and not only that there was one.
func TestARegexMatchReportsWhatItMatched(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), regexCaptureProbe)
	if out != regexCaptureWant || st != 0 {
		t.Errorf("the capture parameters = %q (status %d), want %q", out, st, regexCaptureWant)
	}
}

const regexCaptureProbe = `s='hello world'
[[ $s =~ 'o (w[a-z]+)d' ]]
print -r -- "st=$? MATCH=[$MATCH] MBEGIN=$MBEGIN MEND=$MEND"
print -r -- "match=(${match[@]}) mbegin=(${mbegin[@]}) mend=(${mend[@]}) n=${#match}"
[[ $s =~ 'zzz' ]]
print -r -- "after a failure: st=$? MATCH=[$MATCH] match=(${match[@]}) MBEGIN=$MBEGIN"
[[ $s =~ 'hello' ]]
print -r -- "no groups: MATCH=[$MATCH] MBEGIN=$MBEGIN MEND=$MEND match=(${match[@]})"
[[ abcd =~ 'b(x)?(c)' ]]
print -r -- "unmatched group: n=${#match} [${match[1]}|${match[2]}] mbegin=(${mbegin[@]}) mend=(${mend[@]})"
inner() { [[ abc =~ 'b' ]]; print -r -- "inside: MATCH=[$MATCH]" }
MATCH=; inner; print -r -- "after the call: MATCH=[$MATCH]"
`

const regexCaptureWant = `st=0 MATCH=[o world] MBEGIN=5 MEND=11
match=(worl) mbegin=(7) mend=(10) n=1
after a failure: st=1 MATCH=[o world] match=(worl) MBEGIN=5
no groups: MATCH=[hello] MBEGIN=1 MEND=5 match=(worl)
unmatched group: n=2 [|c] mbegin=(-1 3) mend=(-1 3)
inside: MATCH=[b]
after the call: MATCH=[b]
`
