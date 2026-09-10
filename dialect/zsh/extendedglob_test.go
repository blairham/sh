// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `setopt extendedglob` and the pattern operators behind it. Measured on zsh
// 5.9.2, 2026-09-07, `env -i PATH=/usr/bin:/bin` with a scratch HOME and
// ZDOTDIR — see docs/spec/grammar/patterns.md (#1244).
//
// The name was recorded and not acted on before this, which made every
// construct below read as a literal and every pattern using one quietly fail
// to match. The rows are written as `hit`/`miss` rather than as a status so a
// refusal cannot pass for a miss.

func TestExtendedGlobIsHonoredAndNotOnlyRemembered(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// The same pattern, both ways round: this is the pair that says the
		// option is a switch rather than a formality.
		{`[[ aaa == a# ]] && echo hit || echo miss`, "miss"},
		{`setopt extendedglob; [[ aaa == a# ]] && echo hit || echo miss`, "hit"},
		{`[[ 'a#' == a# ]] && echo hit || echo miss`, "hit"},
		{`setopt extendedglob; [[ 'a#' == a# ]] && echo hit || echo miss`, "miss"},
		// And it goes back off.
		{`setopt extendedglob; unsetopt extendedglob; [[ aaa == a# ]] && echo hit || echo miss`, "miss"},
		// The listing still reports it, which is what a recorded name did.
		{`setopt extendedglob; [[ -o extendedglob ]] && echo hit || echo miss`, "hit"},
	} {
		out, st := runZsh(t, t.TempDir(), tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

func TestTheExtendedOperators(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ ABC == (#i)abc ]] && echo hit || echo miss`, "hit"},
		{`[[ ABCdef == (#i)abc(#I)def ]] && echo hit || echo miss`, "hit"},
		{`[[ ABC == (#l)abc ]] && echo hit || echo miss`, "hit"},
		{`[[ abc == (#l)ABC ]] && echo hit || echo miss`, "miss"},
		{`[[ abc == ^x* ]] && echo hit || echo miss`, "hit"},
		{`[[ acd == a*~*b* ]] && echo hit || echo miss`, "hit"},
		{`[[ abd == a*~*b* ]] && echo hit || echo miss`, "miss"},
		{`[[ ababab == (ab)# ]] && echo hit || echo miss`, "hit"},
		{`[[ aaa == a(#c3) ]] && echo hit || echo miss`, "hit"},
		// A `(#i)` folds a literal and not a bracket, which is where it
		// differs from `nocasematch` and is measured rather than assumed.
		{`[[ ABC == (#i)[abc][abc][abc] ]] && echo hit || echo miss`, "miss"},
	} {
		src := "setopt extendedglob\n" + tc.src
		out, st := runZsh(t, t.TempDir(), src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}

// Every surface, because a flag honored in one place and dropped in another
// is the same silent wrong answer somewhere else.
func TestTheOperatorsReachEverySurfaceInThisShell(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ABC", "abd", "aaa"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ name, src, want string }{
		{"case", `case ABC in ((#i)abc) echo hit;; *) echo miss;; esac`, "hit"},
		{"trim", `v=ABCd; echo "[${v#(#i)abc}]"`, "[d]"},
		{"trim, closure", `v=aaab; echo "[${v##a#}]"`, "[b]"},
		{"replace", `v=xABCy; echo "[${v/(#i)abc/Q}]"`, "[xQy]"},
		{"element exclusion", `a=(ABC def); print -l -- ${a:#(#i)abc}`, "def"},
		{"the (M) filter", `a=(ABC def); print -l -- ${(M)a:#(#i)a*}`, "ABC"},
		{"a (r) subscript", `a=(ABC def); echo "${a[(r)(#i)abc]}"`, "ABC"},
		{"pathname expansion", `echo (#i)ab*`, "ABC abd"},
		{"a qualifier list, (#q…)", `echo *(#q.)`, "ABC aaa abd"},
		// A `(#…)` at the end of the last component is a flag group and not
		// a qualifier list, which is the one place the two spellings could
		// be confused — and were: reading it as a list answered `unknown
		// file attribute: #` for a pattern with no attribute in it, and it
		// is where an end anchor is naturally written.
		{"a flag group at the end", `echo *(#i)ABD`, "abd"},
	} {
		out, st := runZsh(t, dir, "setopt extendedglob\n"+tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s: %s = %q (status %d), want %q", tc.name, tc.src, out, st, tc.want)
		}
	}
	// The `(#q…)` spelling is the option's, and without it the `#` is a
	// letter no qualifier claims — which is this shell's own complaint and
	// was already right.
	out, st := runZsh(t, dir, `echo *(#q.)`)
	if !strings.Contains(out, "unknown file attribute: #") || st == 0 {
		t.Errorf("`*(#q.)` with the option off = %q (status %d), want the attribute complaint", out, st)
	}
}

// The two anchors, in this shell and on every surface it has one.
//
// They are the flags that made the position necessary, and the surfaces
// differ in what they hand the matcher: a condition and a `case` give it the
// whole subject, a trim gives it a prefix or a suffix of one, a replacement
// gives it a span from the middle, and pathname expansion gives it one path
// component at a time. An anchor answered against the *piece* rather than the
// subject would be right on the first two and wrong on the rest.
//
// Measured on zsh 5.9.2, 2026-09-07 — see docs/spec/grammar/patterns.md.
func TestTheAnchorsOnEverySurface(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ax", "bx", "ay"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "cx", "ax"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"condition", `[[ ab == (#s)ab(#e) ]] && echo hit || echo miss`, "hit"},
		{"condition, midway", `[[ ab == a(#s)b ]] && echo hit || echo miss`, "miss"},
		{"case", `case ab in ((#s)ab) echo hit;; *) echo miss;; esac`, "hit"},
		{"case, midway", `case ab in (a(#e)b) echo hit;; *) echo miss;; esac`, "miss"},
		{"trim prefix", `v=abcd; echo "[${v#(#s)ab}]"`, "[cd]"},
		{"trim prefix, end anchor", `v=abcd; echo "[${v#ab(#e)}]"`, "[abcd]"},
		{"trim suffix", `v=abcd; echo "[${v%cd(#e)}]"`, "[ab]"},
		{"trim suffix, start anchor", `v=abcd; echo "[${v%(#s)cd}]"`, "[abcd]"},
		{"replace", `v=XbXcX; echo "[${v//(#s)X/-}]"`, "[-bXcX]"},
		{"replace, end anchor", `v=XbXcX; echo "[${v//X(#e)/-}]"`, "[XbXc-]"},
		{"element exclusion", `a=(ab cb); print -l -- ${a:#(#s)a*}`, "cb"},
		{"the (M) filter", `a=(ab cb); print -l -- ${(M)a:#(#s)a*}`, "ab"},
		{"a (r) subscript", `a=(ab cb); echo "${a[(r)(#s)a*]}"`, "ab"},
		// Per component, which is the answer pathname expansion gives and
		// the one a walk that anchored to the whole path would not: `ax`
		// lies inside `cx`, and its component starts with an `a` even
		// though the path does not.
		{"pathname expansion", `print -l -- (#s)a*`, "ax\nay"},
		{"pathname expansion, component", `print -l -- */(#s)a*`, "cx/ax"},
		{"pathname expansion, end anchor", `print -l -- *x(#e)`, "ax\nbx\ncx"},
		// A numeric range consumes a run the matcher has to measure before
		// it can go on, so the position has to survive it.
		{"a numeric range", `[[ 12ab == (#s)<1-99>ab(#e) ]] && echo hit || echo miss`, "hit"},
	} {
		out, st := runZsh(t, dir, "setopt extendedglob\n"+tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s: %s = %q (status %d), want %q", tc.name, tc.src, out, st, tc.want)
		}
	}
	// With the option off the same characters are a group holding one
	// alternative, which is the pair that says the flags are the option's.
	out, st := runZsh(t, dir, `[[ '#sab' == (#s)ab ]] && echo hit || echo miss`)
	if strings.TrimSpace(out) != "hit" || st != 0 {
		t.Errorf("`(#s)` with the option off = %q (status %d), want hit", out, st)
	}
}

// The reporting flags in this shell, where `$match[1]` is the shell's own
// spelling and its arrays count from one.
//
// Measured on zsh 5.9.2, 2026-09-07 — see docs/spec/grammar/patterns.md.
func TestTheReportingFlagsInThisShell(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ax"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"a condition", `[[ abc == (#b)(a)(b)c ]]; print -r -- "$match[1]$match[2] $mbegin[1]$mbegin[2] $mend[1]$mend[2]"`, "ab 12 12"},
		{"a case arm", `case abc in ((#b)(a)(b)c) print -r -- "$match[1]$match[2]";; esac`, "ab"},
		{"a trim prefix", `v=abcd; print -r -- "${v#(#b)(a)(b)} $match[1]$match[2] $mbegin[2]"`, "cd ab 2"},
		{"a trim suffix", `v=abcd; print -r -- "${v%(#b)(c)(d)} $mbegin[1]$mbegin[2]"`, "ab 34"},
		{"an element filter", `a=(abc xbc); print -r -- ${a:#(#b)(a)*} "$match[1]"`, "xbc a"},
		{"an (r) subscript", `a=(ab cb); print -r -- "${a[(r)(#b)(a)*]} $match[1]"`, "ab a"},
		{"the whole match", `[[ abc == (#m)a* ]]; print -r -- "$MATCH $MBEGIN $MEND"`, "abc 1 3"},
		{"both at once", `[[ abc == (#m)(#b)(a)b* ]]; print -r -- "$MATCH $match[1]"`, "abc a"},
		{"a replacement, per match", `v=abcd; print -r -- "${v//(#b)(b)(c)/<$match[1]-$match[2]>}"`, "a<b-c>d"},
		{"a replacement, the whole match", `v=abcd; print -r -- "${v//(#m)[bc]/<$MATCH:$MBEGIN>}"`, "a<b:2><c:3>d"},
		// A group that never participated, which is the row that says why a
		// quietly dropped flag was never acceptable: empty is a real answer.
		{"a group that did not run", `[[ ac == (#b)(a)((b))#c ]]; print -r -- "[$match[2]] $mbegin[2] $mend[2]"`, "[] -1 -1"},
		// The arrays are the shell's own, so `local` contains them.
		{"local containment", `f() { local match mbegin mend; [[ abc == (#b)(a)* ]]; print -r -- in=$match[1]; }; f; print -r -- "out=[$match[1]]"`, "in=a\nout=[]"},
		// Nothing is written where the pattern asked for nothing.
		{"no group, no write", `match=(zz); [[ abc == (#b)abc ]]; print -r -- "$match[1]"`, "zz"},
		{"no match, no write", `MATCH=zz; [[ abc == (#m)xyz ]]; print -r -- "$MATCH"`, "zz"},
		// Pathname expansion is the surface that reports *nothing*, measured
		// rather than assumed.
		{"the walk reports nothing", `match=(zz); print -rl -- (#b)(a)* >/dev/null; print -r -- "$match[1]"`, "zz"},
	} {
		out, st := runZsh(t, dir, "setopt extendedglob\n"+tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s: %s = %q (status %d), want %q", tc.name, tc.src, out, st, tc.want)
		}
	}
	// The reported positions move with the array base, which is this shell's
	// own option and not a constant: measured, `(#m)` on `abc` reports
	// `1 3` and `0 2` under `ksharrays`.
	const kshArrays = "setopt extendedglob ksharrays\n" +
		"[[ abc == (#m)a* ]]; print -r -- \"$MBEGIN $MEND\""
	out, st := runZsh(t, dir, kshArrays)
	if strings.TrimSpace(out) != "0 2" || st != 0 {
		t.Errorf("`(#m)` under ksharrays = %q (status %d), want 0 2", out, st)
	}
}

// The refusals, which are the half this change is really about: a `(#b)` also
// fills `$match`, so one that was quietly dropped left a script reading
// `$match[1]` with an empty value rather than seeing that the feature is not
// here.
func TestTheUnimplementedFlagsAreRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{`[[ abd == (#a1)abc ]]; echo "m=[$match[1]]"`, "(#a)"},
		{`[[ abc == (#u)abc ]]`, "(#u)"},
		{`[[ abc == (#U)abc ]]`, "(#U)"},
		// Against the filesystem too, where a refusal that reported and
		// carried on would go on to say the pattern matched nothing — a
		// weaker and different claim from the one already made.
		{`echo (#u)a*`, "(#u)"},
	} {
		out, st := runZsh(t, t.TempDir(), "setopt extendedglob\n"+tc.src)
		want := "the " + tc.name + " pattern flag is not implemented"
		if !strings.Contains(out, want) || st == 0 {
			t.Errorf("%s = %q (status %d), want a refusal naming %s", tc.src, out, st, tc.name)
		}
		if strings.Contains(out, "m=[]") {
			t.Errorf("%s printed an empty $match beside its refusal: %q", tc.src, out)
		}
		if strings.Contains(out, "no matches found") {
			t.Errorf("%s said the pattern matched nothing as well as refusing it: %q",
				tc.src, out)
		}
	}
}

// A letter no shell has is this shell's own `bad pattern`, and the status it
// exits with is the surface's rather than one number. Measured all four.
func TestALetterNoShellHasIsThisShellsBadPattern(t *testing.T) {
	for _, tc := range []struct {
		src    string
		status int
	}{
		{`[[ abc == (#Z)abc ]]`, 2},
		{`case abc in ((#Z)abc) echo hit;; esac`, 0},
		{`v=abc; echo "${v#(#Z)a}"`, 1},
		{`echo (#Z)a*`, 1},
		// A `#` at the front of a pattern is the same complaint. It reaches
		// the matcher only through `${~p}`, which is what makes an
		// expansion's characters into a pattern.
		{`p='#foo'; [[ '#foo' == ${~p} ]]`, 2},
	} {
		out, st := runZsh(t, t.TempDir(), "setopt extendedglob\n"+tc.src)
		if !strings.Contains(out, "bad pattern") || st != tc.status {
			t.Errorf("%s = %q (status %d), want `bad pattern` at %d",
				tc.src, out, st, tc.status)
		}
	}
}

// A metacharacter that arrived from a value is not one, which is the answer
// this shell already gives for `*` asked of the three new characters.
func TestTheOperatorsAreNotReadInAnExpansionsResult(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`p="a#b"; [[ ab == $p ]] && echo hit || echo miss`, "miss"},
		{`p="a#b"; [[ 'a#b' == $p ]] && echo hit || echo miss`, "hit"},
		{`p="a#b"; [[ ab == ${~p} ]] && echo hit || echo miss`, "hit"},
		{`p="^x"; [[ ab == $p ]] && echo hit || echo miss`, "miss"},
		{`p="^x"; [[ ab == ${~p} ]] && echo hit || echo miss`, "hit"},
		// The dead half of the pair above: the same `#foo` is a pattern that
		// is refused through `${~p}` and four ordinary characters through
		// `$p`, and this is the second half.
		{`p='#foo'; [[ '#foo' == $p ]] && echo hit || echo miss`, "hit"},
	} {
		out, st := runZsh(t, t.TempDir(), "setopt extendedglob\n"+tc.src)
		if strings.TrimSpace(out) != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
		}
	}
}
