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

// The refusals, which are the half this change is really about: a `(#b)` also
// fills `$match`, so one that was quietly dropped left a script reading
// `$match[1]` with an empty value rather than seeing that the feature is not
// here.
func TestTheUnimplementedFlagsAreRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{`[[ abc == (#b)(a)* ]]; echo "m=[$match[1]]"`, "(#b)"},
		{`[[ abc == (#B)(a)* ]]`, "(#B)"},
		{`[[ abc == (#m)a* ]]`, "(#m)"},
		{`[[ abd == (#a1)abc ]]`, "(#a)"},
		// Against the filesystem too, where a refusal that reported and
		// carried on would go on to say the pattern matched nothing — a
		// weaker and different claim from the one already made.
		{`echo (#m)a*`, "(#m)"},
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
	// An exclusion that would have to span a path component is refused for
	// the same reason: this walk reads one component at a time, and running
	// the right side against a file's name alone would answer the wrong
	// question quietly.
	out, st := runZsh(t, t.TempDir(), "setopt extendedglob\necho **/x~*bar*")
	if !strings.Contains(out, "exclusion spanning a path component") || st == 0 {
		t.Errorf("a cross-component exclusion = %q (status %d), want a refusal", out, st)
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
