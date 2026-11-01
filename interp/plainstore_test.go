// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainStoreScript writes a bare assignment over every state the plain store
// must either answer exactly as the full road does or leave to it: a frozen
// name, the integer and case attributes, a reference, arrays of each kind, a
// produced parameter, allexport, `set -u`, a function's local and a declared
// one with no value, an unset name coming back, the names the store treats
// specially, and values with a tilde, an equals sign, quoting and escapes.
//
// Shapes the vector refuses are run through `command eval`, so a refusal is
// part of the output both runs must agree on and the script carries on.
const plainStoreScript = `p() { printf '[%s]' "$@"; echo; }
i=0; i=$((i + 1)); i=$((i * 7)); p "$i"; qa="$((i - 4))"; p "$qa"; ne=$((-i < 0 ? i : 0)); p "$ne"
xr='i=9'; i=$((xr)); p "$i"; jr='k=5'; k=1; k=$((jr + 10)); p "$k"; ch=jr; ch=$((ch)); p "$ch" "$k"
(dz=4; dz=$((1 / 0)); echo "dz=$dz"); echo "<$?>"
command eval 'readonly roa=1'; command eval 'roa=$((roa + 1))'; echo "<$?>"; p "$roa"
command eval 'typeset -i ni; ni=$((2 * 3))'; p "$ni"; command eval 'typeset -u ua; ua=$((10))'; p "$ua"
command eval 'ar=(4 5)'; (av=$((ar + 1)); p "$av"); ca=$((COUNT + 0)); p "$ca"; un=$((nosuch5 + 2)); p "$un"
(set -u; uv=$((nosuch4 + 1)); echo "uv=$uv"); echo "<$?>"
x=; p "$x"; x=0; p "$x"; y=abc; x=$y; p "$x"; x=$nosuch; p "$x"
command eval 'readonly ro=1'; command eval 'ro=2'; echo "<$?>"; p "$ro"
command eval 'typeset -i n; n=3+4'; p "$n"
command eval 'typeset -u up; up=abc'; p "$up"
command eval 'typeset -l lo; lo=ABC'; p "$lo"
command eval 'typeset -n ref=y'; command eval 'ref=viaref'; p "$y"
command eval 'a=(1 2 3)'; a=v; p "$a" "${a[1]}"
command eval 'typeset -A m; m[k]=v'; command eval 'm=w'; p "${m[k]}" "${m[0]}"
COUNT=5; p "$COUNT" "$COUNT"
set -a; ex=exported; set +a; case "$(export -p)" in *ex=*) echo ex=exported;; esac
unset gone; gone=back; p "$gone"
f() { local l; l=in; p "$l"; g=global; local d; d=v; p "$d"; }; f; p "$l" "$g"
OPTIND=3; p "$OPTIND"; PATH=$PATH; IFS=:; v=a:b; p $v; unset IFS
HOME=/h; t=~; p "$t"; e=a=b; p "$e"; q='a b'; p "$q"; z=$'t\tt'; p "$z"
(set -u; w=$nosuch2; echo "w=$w"); echo "<$?>"
tr=tq/zv; x=${tr#*/}; p "$x"; x=${tr%%/*}; p "$x"; x=${tr##*[/]}; p "$x"; x=${tr%"/zv"}; p "$x"
command eval 'ro=${tr#t}'; echo "<$?>"; p "$ro"
command eval 'typeset -i n2; e2=1+2; n2=${e2#0}'; p "$n2"
command eval 'typeset -u up2; up2=${tr#t}'; p "$up2"
command eval 'typeset -n ref2=tr2'; command eval 'ref2=${tr#t}'; p "$tr2"
command eval 'a2=(1 2 3)'; a2=${tr#t}; p "$a2" "${a2[1]}"
command eval 'arr=(x/1 y/2)'; x=${arr#*/}; p "$x"; x=${COUNT#1}; p "$x"; x=${tr#~}; p "$x"
(set -u; w=${nosuch3#a}; echo "w=$w"); echo "<$?>"
`

// TestAPlainScalarStoreAnswersAsTheFullRoadDoes runs plainStoreScript with
// the plain store on and off, and requires the same output and status
// (#6423).
func TestAPlainScalarStoreAnswersAsTheFullRoadDoes(t *testing.T) {
	f, err := syntax.Parse(plainStoreScript, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	for name, sem := range plainParamVectors() {
		t.Run(name, func(t *testing.T) {
			run := func(on bool) (string, int) {
				was := SetPlainScalarStoreShortcutForTest(on)
				defer SetPlainScalarStoreShortcutForTest(was)
				var out strings.Builder
				r := plainParamRunner(t, sem, &out)
				st, err := r.Run(context.Background(), f)
				if err != nil {
					t.Fatalf("shortcut %v: %v", on, err)
				}
				return out.String(), st
			}
			full, fullStatus := run(false)
			short, shortStatus := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)",
					short, shortStatus, full, fullStatus)
			}
			// The rows the comparison leans on, so a script that stopped
			// early compares nothing.
			for _, row := range []string{"[7]\n[3]\n[7]\n", "[9]\n", "[15]\n", "[2]\n", "[abc]\n", "[7]\n", "[ABC]\n", "[abc]\n", "ex=exported\n", "[in]\n", "[back]\n", "[3]\n", "[/h]\n", "[a=b]\n", "[a b]\n", "[zv]\n", "[tq]\n", "[Q/ZV]\n"} {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

// plainDeclarationScript declares names with no value over the states a
// valueless declaration's store must answer exactly as the full road does or
// leave to it, in a vector where such a declaration sets the name to the empty
// string: an unset name, a set one, an exported one, the integer and case
// attributes, an array, a produced parameter, allexport, a redeclaration, the
// names the store treats specially, and a reference standing anywhere, which
// comes last because it turns the shortcut off for everything after it.
const plainDeclarationScript = `p() { printf '[%s]' "$@"; echo; }
dv=outer; de=exp; export de; typeset -i di=3; typeset -u du=abc; command eval 'da=(1 2)'
f() { local dn dv de di du da; p "<${dn-unset}>" "<$dv>" "<$de>" "<$di>" "<$du>" "<$da>"
  local dv; p "<$dv>"; di=2+3; du=xyz; p "$di" "$du"; set -a; local dx; set +a
  case "$(export -p)" in *dx=*) echo dx-exported;; *de=*) echo de-exported;; *) echo none-exported;; esac
  local COUNT OPTIND PATH IFS; p "<$COUNT>" "<$OPTIND>" "<${PATH-unset}>" "<${IFS-unset}>"; }
f; p "$dv" "$de" "$di" "$du" "<${dn-unset}>" "${da[1]}"
g() { local q; q=in; p "$q"; local q; p "<$q>"; }; g; p "<${q-unset}>"
command eval 'typeset -n ref=dv'; h() { local dv; p "<$dv>" "<$ref>"; }; h; p "$dv"
`

// TestAPlainDeclarationStoreAnswersAsTheFullRoadDoes runs
// plainDeclarationScript with the plain store on and off, and requires the
// same output and status (#6373).
func TestAPlainDeclarationStoreAnswersAsTheFullRoadDoes(t *testing.T) {
	f, err := syntax.Parse(plainDeclarationScript, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := plainParamVectors()["first element"]
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	run := func(on bool) (string, int) {
		was := SetPlainScalarStoreShortcutForTest(on)
		defer SetPlainScalarStoreShortcutForTest(was)
		var out strings.Builder
		r := plainParamRunner(t, sem, &out)
		st, err := r.Run(context.Background(), f)
		if err != nil {
			t.Fatalf("shortcut %v: %v", on, err)
		}
		return out.String(), st
	}
	full, fullStatus := run(false)
	short, shortStatus := run(true)
	if short != full || shortStatus != fullStatus {
		t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
	}
	// The rows the comparison leans on: a valueless local holding the empty
	// string rather than nothing, which is the branch the store is on, and
	// the outer values back after the return.
	for _, row := range []string{
		"[<>][<>][<>][<>][<>]", "[<>]\n", "[outer][exp][3][ABC][<unset>]", "[in]\n[<in>]\n[<unset>]\n",
	} {
		if !strings.Contains(full, row) {
			t.Errorf("the script did not reach %q:\n%s", row, full)
		}
	}
}

// TestThePlainScalarStoreIsReached says the comparison above compares
// something: a bare assignment of a plain value to an ordinary name is
// answered by the shortcut, and the states it must leave to the full road are
// left to it.
func TestThePlainScalarStoreIsReached(t *testing.T) {
	sem := plainParamVectors()["first element"]
	var out strings.Builder
	r := plainParamRunner(t, sem, &out)
	run := func(src string) {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if st, err := r.Run(context.Background(), f); st != 0 || err != nil {
			t.Fatalf("%q: %d %v %s", src, st, err, out.String())
		}
	}
	assign := func(src string) *syntax.Assign {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		return f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd).Assigns[0]
	}
	run("plain=1; y=abc; readonly ro=1; typeset -i n; a=(1 2)")
	// An arithmetic value is taken on its text and handed to the full road
	// when a value it read was read again as an expression, which can write.
	for src, want := range map[string]int{"plain=$((plain + 1))": 0, "xr='q=9'; plain=$((xr))": 1, "plain=$((y))": 1} {
		before := PlainStoreHandedOnForTest(r)
		run(src)
		if got := PlainStoreHandedOnForTest(r) - before; got != want {
			t.Errorf("%s: handed %d assignments to the full road, want %d", src, got, want)
		}
	}
	for src, want := range map[string]bool{
		"plain=":                    true,
		"plain=2":                   true,
		"fresh=x":                   true,
		"plain=$y":                  true,
		"plain=${y#a}":              true,
		"plain=${y%%*c}":            true,
		"plain=${y#'a'}":            true,
		"plain=\"${y#a}\"":          false,
		"plain=${y#~}":              false,
		"plain=${y#$y}":             false,
		"plain=${y:-a}":             false,
		"plain=${a#1}":              false,
		"plain=${y[1]#a}":           false,
		"plain='q'":                 true,
		"plain=$((plain + 1))":      true,
		"plain=\"$((plain * 2))\"":  true,
		"plain=$((plain ? 1 : -y))": true,
		"plain=$((plain++))":        false,
		"plain=$((plain = 2))":      false,
		"plain=$((plain += 2))":     false,
		"plain=$((a + 1))":          false,
		"plain=$((COUNT))":          false,
		"plain=$(($y + 1))":         false,
		"plain=$((a[0]))":           false,
		"n=$((1))":                  false,
		"plain=a=b":                 false,
		"plain=~":                   false,
		"plain=$(:)":                false,
		"plain=a$y":                 false,
		"plain+=x":                  false,
		"ro=2":                      false,
		"n=3":                       false,
		"a=x":                       false,
		"COUNT=1":                   false,
		"PATH=x":                    false,
		"OPTIND=1":                  false,
	} {
		if got := StoresAPlainScalarForTest(r, assign(src)); got != want {
			t.Errorf("%s: the plain store answers %v, want %v", src, got, want)
		}
	}
}
