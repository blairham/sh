// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/blairham/sh/interp"
)

// The debug surface can refuse every kind of action the gate defines, and a
// seventh kind cannot land without one.
//
// This is the half of #520 worth having. The gap it closes was small and
// invisible: `-deny` held a list of paths, `ActionSignal` is the one kind with
// no path, so `-trace-events` could watch a signal and nothing shipped could
// refuse one. Nothing failed. #460's argument is that a seam no binary reaches
// is a seam nothing grades, and a *hole* in the surface that reaches it has the
// same property — it looks exactly like a shell that works.
//
// A vocabulary without this guard only moves the gap: the next kind arrives,
// the surface silently cannot express it, and the same sentence gets written
// again. So the list of kinds is not written here. It is read out of
// interp/seams.go, which is the declaration and therefore the only thing that
// cannot be forgotten to update.

// seamsFile is where the action vocabulary is declared.
//
// A path rather than a symbol, because the point is to read the *declaration*.
// A Go-level list — a slice in interp naming every kind — would be a second
// list, and a seventh kind added without touching it would defeat exactly the
// check this is.
const seamsFile = "../../interp/seams.go"

// denyable is how each action kind is refused through -deny, and a sample
// action of that kind to prove it.
//
// Keyed by the constant's name in the source, so a kind that arrives without an
// entry is named in the failure by the name its author typed.
var denyable = map[string]struct {
	value  string
	action interp.Action
}{
	"ActionExec": {
		"exec:/secret/**",
		interp.Action{Kind: interp.ActionExec, Path: "/secret/prog"},
	},
	"ActionOpen": {
		"open:/secret/**",
		interp.Action{Kind: interp.ActionOpen, Path: "/secret/f"},
	},
	"ActionStat": {
		"stat:/secret/**",
		interp.Action{Kind: interp.ActionStat, Path: "/secret/f"},
	},
	"ActionReadDir": {
		"list:/secret/**",
		interp.Action{Kind: interp.ActionReadDir, Path: "/secret"},
	},
	// The kind that started this. It carries no path, so no rule a path list
	// could hold would ever match one.
	"ActionSignal": {
		"signal",
		interp.Action{Kind: interp.ActionSignal, PID: 4321, Signal: syscall.SIGTERM},
	},
}

// exempt names a kind that must *not* be refusable, with the reason, because
// "it cannot be denied" and "nobody got round to it" look identical from here.
//
// Adding to this is a decision and reads as one in a diff. Deleting an entry
// from `denyable` and adding it here would be the way to hide a gap, and it is
// the shape a reviewer can see.
var exempt = map[string]string{
	"ActionInherit": "recorded and never gated — a descriptor the shell was " +
		"handed is already in the process's table, so a veto would hide it " +
		"from the script and leave it reachable. See interp/seams.go.",
}

// unaccounted names the kinds that neither table speaks for, and the ones both
// speak for, which are the two ways this guard goes wrong.
//
// Split out from the test so the test below can hand it a vocabulary of its
// own. A guard that can only be pointed at the tree it guards is a guard whose
// own failure looks exactly like success — which is not hypothetical here:
// reporting a missing kind as a skip left the whole suite green, and the guard
// would then have watched a seventh kind land in the silence it exists to
// break. A mutation is what said so.
// The tables are passed in rather than read from the package, so that a case
// this vocabulary does not currently have — a kind in *both* — can be put to it
// anyway. A detector whose second answer nothing exercises is half a detector,
// and this one's second answer went unexercised until a mutation removed it and
// nothing failed.
func unaccounted(kinds, canDeny, isExempt []string) (missing, both []string) {
	for _, name := range kinds {
		deniable := slices.Contains(canDeny, name)
		excused := slices.Contains(isExempt, name)
		switch {
		case deniable && excused:
			both = append(both, name)
		case !deniable && !excused:
			missing = append(missing, name)
		}
	}
	return missing, both
}

// tables is the two lists as the guard actually holds them.
func tables() (canDeny, isExempt []string) {
	return slices.Sorted(maps.Keys(denyable)), slices.Sorted(maps.Keys(exempt))
}

func TestEveryActionKindCanBeDenied(t *testing.T) {
	kinds, err := actionKinds(seamsFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) < 6 {
		t.Fatalf("found %d action kinds in %s, want the whole block — the guard is looking in the wrong place",
			len(kinds), seamsFile)
	}
	canDeny, isExempt := tables()
	missing, both := unaccounted(kinds, canDeny, isExempt)
	if len(missing) != 0 {
		t.Errorf("%v can be watched and not refused.\n"+
			"Every kind the gate defines needs a way to say no through -deny, or the "+
			"debug surface has a hole that looks exactly like a shell that works — "+
			"which is what ActionSignal was.\n"+
			"Add a -deny value for it here, or, if it genuinely must not be gated, "+
			"add it to exempt with the reason.", missing)
	}
	if len(both) != 0 {
		t.Errorf("%v are both denyable and exempt: pick one", both)
	}

	for _, name := range kinds {
		tc, ok := denyable[name]
		if !ok {
			continue
		}
		t.Run(name, func(t *testing.T) {
			g, err := denyRules([]string{tc.value})
			if err != nil {
				t.Fatalf("-deny %s: %v", tc.value, err)
			}
			if got := g.Allow(context.Background(), tc.action); got != interp.Deny {
				t.Errorf("-deny %s answered %v for %+v, want Deny", tc.value, got, tc.action)
			}
			// And the same kind is *allowed* when the rule does not name it,
			// so a gate that refused everything would not pass this by
			// accident. That is the failure a coverage test is most likely to
			// have: it proves a denial and not that the denial was the rule's.
			other, err := denyRules([]string{"/nowhere-this-test-uses"})
			if err != nil {
				t.Fatal(err)
			}
			if got := other.Allow(context.Background(), tc.action); got != interp.Allow {
				t.Errorf("an unrelated -deny answered %v for %+v, want Allow", got, tc.action)
			}
		})
	}
}

// actionKinds reads the names declared in interp's ActionKind constant block.
//
// The block is found by the type on its first spec — `ActionExec ActionKind =
// iota` — rather than by position or by a name prefix, so reordering the file
// or adding a kind whose name does not start with "Action" does not lose it.
func actionKinds(path string) ([]string, error) {
	src, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		if !isActionKindBlock(gen) {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				if n.Name != "_" {
					names = append(names, n.Name)
				}
			}
		}
	}
	slices.Sort(names)
	return names, nil
}

// isActionKindBlock reports whether a const block declares ActionKind values,
// which is true when its first spec names that type.
func isActionKindBlock(gen *ast.GenDecl) bool {
	for _, spec := range gen.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		id, ok := vs.Type.(*ast.Ident)
		return ok && id.Name == "ActionKind"
	}
	return false
}

// The guard catches what it is for.
//
// A guard over a vocabulary that already obeys the rule reports nothing whether
// it works or not, so the passing run above is not evidence. These are the two
// halves of the mechanism written out: that a seventh kind is *found* in the
// declaration, and that a found kind nothing speaks for is *reported*.
func TestTheGuardReportsAKindNothingSpeaksFor(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		kinds, canDeny, isExempt []string
		wantMissing, wantBoth    []string
	}{
		{
			"a kind nothing speaks for is reported",
			[]string{"A", "B", "C"},
			[]string{"A"},
			[]string{"B"},
			[]string{"C"},
			nil,
		},
		{
			"a vocabulary fully spoken for reports nothing",
			[]string{"A", "B"},
			[]string{"A"},
			[]string{"B"},
			nil, nil,
		},
		{
			// Not a state this vocabulary is in, and that is the reason to put
			// it here: an exemption added beside an entry that already exists
			// would otherwise read as coverage while saying the kind must not
			// be gated, which is two opposite claims about one kind.
			"a kind in both tables is reported",
			[]string{"A"},
			[]string{"A"},
			[]string{"A"},
			nil,
			[]string{"A"},
		},
		{
			"an empty vocabulary reports nothing",
			nil,
			[]string{"A"},
			[]string{"B"},
			nil, nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			missing, both := unaccounted(tc.kinds, tc.canDeny, tc.isExempt)
			if !slices.Equal(missing, tc.wantMissing) {
				t.Errorf("missing = %v, want %v", missing, tc.wantMissing)
			}
			if !slices.Equal(both, tc.wantBoth) {
				t.Errorf("both = %v, want %v", both, tc.wantBoth)
			}
		})
	}
}

func TestTheGuardSeesASeventhKind(t *testing.T) {
	const src = `package interp

type ActionKind uint8

const (
	ActionExec ActionKind = iota
	ActionOpen
	ActionStat
	ActionReadDir
	ActionSignal
	ActionInherit
	ActionConnect
)

const (
	other = 1
)
`
	dir := t.TempDir()
	path := filepath.Join(dir, "seams.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := actionKinds(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "ActionConnect") {
		t.Errorf("found %v, want the seventh kind among them", got)
	}
	// And nothing from the unrelated const block, or the guard would demand a
	// -deny value for every constant in the file.
	if slices.Contains(got, "other") {
		t.Errorf("found %v, want only the ActionKind block", got)
	}
	if len(got) != 7 {
		t.Errorf("found %d names (%v), want the seven declared", len(got), got)
	}
}

// Every kind the guard knows about is spelled the way interp spells it.
//
// The table above is keyed by the constant's name in the source, and a typo in
// a key would silently exempt a kind: the guard would report it missing, the
// next person would add a second entry, and the first would sit there meaning
// nothing. This catches the typo instead.
func TestTheGuardsTableNamesRealKinds(t *testing.T) {
	kinds, err := actionKinds(seamsFile)
	if err != nil {
		t.Fatal(err)
	}
	canDeny, isExempt := tables()
	for _, name := range slices.Concat(canDeny, isExempt) {
		if !slices.Contains(kinds, name) {
			t.Errorf("the guard names %q, which is not an ActionKind in %s: %v",
				name, seamsFile, kinds)
		}
	}
}

// The shorthand and the written-out form are the same rule.
//
// `-deny /etc` is documented as `deny path /etc/**`, and a shorthand that meant
// something slightly different from what it is documented as would be the
// second vocabulary this change exists to remove, reintroduced as a special
// case.
func TestTheBarePathShorthandIsTheWrittenOutRule(t *testing.T) {
	for _, tc := range []struct{ short, long string }{
		{"/etc", "path:/etc/**"},
		{"/var/lib/", "path:/var/lib/**"},
	} {
		t.Run(tc.short, func(t *testing.T) {
			a, err := denyRules([]string{tc.short})
			if err != nil {
				t.Fatal(err)
			}
			b, err := denyRules([]string{tc.long})
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{
				"/etc", "/etc/passwd", "/etcetera", "/var/lib", "/var/lib/x", "/var/libexec",
			} {
				for _, k := range []interp.ActionKind{
					interp.ActionExec, interp.ActionOpen, interp.ActionStat, interp.ActionReadDir,
				} {
					act := interp.Action{Kind: k, Path: path}
					if a.Allow(context.Background(), act) != b.Allow(context.Background(), act) {
						t.Errorf("%s and %s disagree about %v %s", tc.short, tc.long, k, path)
					}
				}
			}
		})
	}
}

// The values the documentation shows are values the flag accepts.
//
// A worked example in a comment that the code refuses is worse than no example,
// because it is the first thing anybody copies.
func TestTheDocumentedDenyValuesParse(t *testing.T) {
	src, err := os.ReadFile("trace.go")
	if err != nil {
		t.Fatal(err)
	}
	var shown []string
	for _, line := range strings.Split(string(src), "\n") {
		_, after, ok := strings.Cut(line, "//\t-deny ")
		if !ok {
			continue
		}
		value, _, _ := strings.Cut(after, " ")
		shown = append(shown, value)
	}
	if len(shown) < 4 {
		t.Fatalf("found %d documented -deny values (%v), want the worked examples", len(shown), shown)
	}
	for _, v := range shown {
		if _, err := denyRules([]string{v}); err != nil {
			t.Errorf("-deny %s is documented and refused: %v", v, err)
		}
	}
}
