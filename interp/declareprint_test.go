// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `declare -p` and `typeset -p` write a declaration back. The shape is an
// axis (DeclareListing), the value spelling a second (DeclareValueQuoting),
// and whether a missing name is worth a message a third
// (DeclarePrintReportsAMissingName). These tests name the axes, never a
// shell.

// declareRun runs src with the given answers over an empty environment, so a
// listing sees only what the snippet made.
func declareRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := testSemantics()
	if set != nil {
		set(&sem)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg,
		Name: "testsh", Env: []string{},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

// TestDeclarePrintClusteredForm is the form with one command word, one flag
// cluster and `--` standing where there is no attribute, paired with the
// always-double value spelling.
func TestDeclarePrintClusteredForm(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`v=1; typeset -p v`, `declare -- v="1"`},
		{`v=; typeset -p v`, `declare -- v=""`},
		{`v='a b'; typeset -p v`, `declare -- v="a b"`},
		// The characters that are live inside double quotes are escaped; a
		// control character forces the dollar spelling instead.
		{`v='a"b'; typeset -p v`, `declare -- v="a\"b"`},
		{`v='$c\d'; typeset -p v`, `declare -- v="\$c\\d"`},
		{`v=$'a\tb'; typeset -p v`, `declare -- v=$'a\tb'`},
		{`export e=E; typeset -p e`, `declare -x e="E"`},
		{`typeset -r r=R; typeset -p r`, `declare -r r="R"`},
		// The attribute evaluated the value, and the listing shows the
		// result.
		{`typeset -i n=5+2; typeset -p n`, `declare -i n="7"`},
		{`typeset -i n=5; typeset -r n; export n; typeset -p n`, `declare -irx n="5"`},
		// An attribute with no value lists without the `=`.
		{`export u; typeset -p u`, `declare -x u`},
		// Names print in the order given, each on its own line.
		{`b=2; a=1; typeset -p b a`, "declare -- b=\"2\"\ndeclare -- a=\"1\""},
	} {
		out, errs, st := declareRun(t, tc.src, nil, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				tc.src, out, errs, st, tc.want)
		}
	}
}

// TestDeclarePrintClusteredArrays: subscripts always written, one trailing
// space per associative element, and an empty associative table listed with
// no value at all — the attribute is the whole of what it has to say.
func TestDeclarePrintClusteredArrays(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`arr=(x y); typeset -p arr`, `declare -a arr=([0]="x" [1]="y")`},
		{`arr=(); typeset -p arr`, `declare -a arr=()`},
		{`arr=(x); arr[3]=z; typeset -p arr`, `declare -a arr=([0]="x" [3]="z")`},
		{`arr=("a b" ""); typeset -p arr`, `declare -a arr=([0]="a b" [1]="")`},
		// Keys are listed sorted: the shells promise no order at all, so the
		// deterministic one is this implementation's to choose.
		{`typeset -A m; m[b]=2; m[a]=1; typeset -p m`, `declare -A m=([a]="1" [b]="2" )`},
		// A key quotes only when it must; a value always.
		{`typeset -A m; m=(["x y"]=2); typeset -p m`, `declare -A m=(["x y"]="2" )`},
		{`typeset -A m; typeset -p m`, `declare -A m`},
		{`arr=(x); export arr; typeset -p arr`, `declare -ax arr=([0]="x")`},
	} {
		out, errs, st := declareRun(t, tc.src, nil, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				tc.src, out, errs, st, tc.want)
		}
	}
}

// TestDeclarePrintExportSpelledForm: `typeset` unless the name is an exported
// scalar, which is spelled `export` with the `x` dropped; arrays wrapped in
// padded parentheses with no subscripts, a gap being an empty element.
func TestDeclarePrintExportSpelledForm(t *testing.T) {
	form := func(s *Semantics) {
		s.DeclareListing = DeclareListingExportSpelled
		s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
		// The form belongs to an engine whose arrays are dense.
		s.ArraysAreSparse = No
	}
	for _, tc := range []struct{ src, want string }{
		{`v=1; typeset -p v`, `typeset v=1`},
		{`v='a b'; typeset -p v`, `typeset v='a b'`},
		{`v=$'a\tb'; typeset -p v`, `typeset v=$'a\tb'`},
		{`export e=E; typeset -p e`, `export e=E`},
		{`export e=E; typeset -r e; typeset -p e`, `export -r e=E`},
		{`typeset -r r=R; typeset -p r`, `typeset -r r=R`},
		{`typeset -i n=5+2; typeset -p n`, `typeset -i n=7`},
		{`arr=(x y); typeset -p arr`, `typeset -a arr=( x y )`},
		{`arr=(); typeset -p arr`, `typeset -a arr=(  )`},
		{`arr=(x); arr[3]=z; typeset -p arr`, `typeset -a arr=( x '' '' z )`},
		// An exported array keeps the word and the letter — only a scalar
		// earns the `export` spelling.
		{`arr=(x); export arr; typeset -p arr`, `typeset -ax arr=( x )`},
		{`typeset -A m; m[b]=2; m[a]=1; typeset -p m`, `typeset -A m=( [a]=1 [b]=2 )`},
		{`typeset -A m; m=(["x y"]=2); typeset -p m`, `typeset -A m=( ['x y']=2 )`},
		{`typeset -A m; typeset -p m`, `typeset -A m=( )`},
	} {
		out, errs, st := declareRun(t, tc.src, form, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				tc.src, out, errs, st, tc.want)
		}
	}
}

// TestDeclarePrintBareAssignmentsForm: each flag its own word, and a name
// with no attributes written as a bare assignment with no command word.
func TestDeclarePrintBareAssignmentsForm(t *testing.T) {
	form := func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		s.DeclarePrintReportsAMissingName = No
	}
	for _, tc := range []struct{ src, want string }{
		{`v=1; typeset -p v`, `v=1`},
		{`v='a b'; typeset -p v`, `v='a b'`},
		{`v=$'it\'s'; typeset -p v`, `v=$'it\'s'`},
		{`export e=E; typeset -p e`, `typeset -x e=E`},
		// Not the clustered order: export first, the kind last.
		{`typeset -i n=5; typeset -r n; export n; typeset -p n`, `typeset -x -r -i n=5`},
		{`typeset -i n; typeset -p n`, `typeset -i n`},
		// Subscripts appear only where they carry information.
		{`arr=(x y); typeset -p arr`, `typeset -a arr=(x y)`},
		{`arr=(x); arr[3]=z; typeset -p arr`, `typeset -a arr=([0]=x [3]=z)`},
		{`arr=(); typeset -p arr`, `typeset -a arr=()`},
		{`typeset -A m; m[b]=2; m[a]=1; typeset -p m`, `typeset -A m=([a]=1 [b]=2)`},
		{`typeset -A m; m=(["x y"]=2); typeset -p m`, `typeset -A m=(['x y']=2)`},
		{`typeset -A m; typeset -p m`, `typeset -A m=()`},
	} {
		out, errs, st := declareRun(t, tc.src, form, Diagnostics{})
		if strings.TrimSuffix(out, "\n") != tc.want || st != 0 || errs != "" {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				tc.src, out, errs, st, tc.want)
		}
	}
}

// TestDeclarePrintMissingName pins both sides of the axis: reported with the
// dialect's wording and status 1, or passed over in silence with status 0 —
// and the other names list either way.
func TestDeclarePrintMissingName(t *testing.T) {
	src := `v=1; typeset -p nosuch v`

	out, errs, st := declareRun(t, src, nil, Diagnostics{})
	if !strings.Contains(errs, "typeset: nosuch: not found") {
		t.Errorf("reported: stderr %q, want the substrate wording behind the builtin's name", errs)
	}
	if st != 1 {
		t.Errorf("reported: status %d, want 1", st)
	}
	if !strings.Contains(out, `declare -- v="1"`) {
		t.Errorf("reported: stdout %q, want the found name listed anyway", out)
	}

	worded := Diagnostics{DeclareNoSuchVariable: "no such variable: %[1]s"}
	_, errs, _ = declareRun(t, src, nil, worded)
	if !strings.Contains(errs, "no such variable: nosuch") {
		t.Errorf("worded: stderr %q, want the dialect's wording", errs)
	}

	silent := func(s *Semantics) { s.DeclarePrintReportsAMissingName = No }
	out, errs, st = declareRun(t, src, silent, Diagnostics{})
	if errs != "" || st != 0 {
		t.Errorf("silent: stderr %q status %d, want nothing and 0", errs, st)
	}
	if !strings.Contains(out, `v="1"`) {
		t.Errorf("silent: stdout %q, want the found name listed", out)
	}
}

// TestDeclarePrintWithNoNamesListsEverythingSorted: the runner's own tables
// and the environment the shell was born with, one sorted listing. Two of the
// three engines sort and the third promises nothing, so sorted is the
// deterministic choice.
func TestDeclarePrintWithNoNamesListsEverythingSorted(t *testing.T) {
	f, err := syntax.Parse(`b=2; a=1; typeset -A m; m[k]=v; typeset -p`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := testSemantics()
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Name: "testsh",
		// Born in the environment: listed as exported, in its sorted place.
		Env: []string{"ZED=z"},
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil || st != 0 {
		t.Fatalf("run: %v, status %d", rerr, st)
	}
	// The runner seeds a few names of its own (IFS, PWD, …), so the listing
	// is checked for order and presence rather than as one exact string:
	// ZED sorts before the lowercase names, which crosses the environment,
	// the scalar table and the associative one in a single comparison.
	listing := o.String()
	last := -1
	for _, line := range []string{
		"declare -x ZED=\"z\"\n",
		"declare -- a=\"1\"\n",
		"declare -- b=\"2\"\n",
		"declare -A m=([k]=\"v\" )\n",
	} {
		i := strings.Index(listing, line)
		if i < 0 {
			t.Fatalf("listing %q is missing %q", listing, line)
		}
		if i < last {
			t.Errorf("listing %q holds %q out of sorted order", listing, line)
		}
		last = i
	}
}

// TestDeclarePrintRefusedWithoutADialect: the core answers no listing
// question of its own, so `-p` under it is a refusal rather than some shell's
// format by default.
func TestDeclarePrintRefusedWithoutADialect(t *testing.T) {
	f, err := syntax.Parse(`v=1; typeset -p v`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var o, e bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &o, Stderr: &e, Name: "testsh", Env: []string{}})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if st != 2 || !strings.Contains(e.String(), "no dialect was chosen") {
		t.Errorf("status %d, stderr %q; want 2 and a refusal naming the axis", st, e.String())
	}
	if o.String() != "" {
		t.Errorf("stdout %q, want no listing in any shell's format", o.String())
	}
}
