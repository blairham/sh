// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/terminfofixture"
)

// Reading a terminal description, against descriptions this file builds.
//
// Built rather than taken from the machine, and that is the whole design of
// these tests. /usr/share/terminfo is not the same on a laptop and a runner —
// it is not even guaranteed to exist — so a test that read it would assert
// whatever the machine happened to hold, and `$TERM` is environment like any
// other: a test that let the developer's own through would pass here and
// answer differently somewhere else. Every test below points `$TERMINFO` at a
// directory it wrote, with a `$TERM` it chose.
//
// The values are real ones. `cuu1` is `\e[A` because that is what
// `xterm-256color` holds, measured with `od -An -tx1` — `1b 5b 41` — and the
// point of #2076 is the bytes, so the bytes are what is asserted.

// database writes a database of descriptions and answers an environment that
// finds `term` in it, and nothing else — no `$HOME`, no `$TERMINFO_DIRS`, and
// not the machine's own `$TERM`.
func database(t *testing.T, term string, ds ...terminfofixture.Description) func(string) string {
	t.Helper()
	return environment(map[string]string{
		"TERM": term, "TERMINFO": terminfofixture.Database(t, ds...),
	})
}

// environment answers variables from a map and nothing from the process, so
// that no test reads the machine it runs on.
func environment(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// capabilitiesByName is the reading a caller does: every capability under its
// terminfo name.
func capabilitiesByName(env func(string) string) map[string]string {
	out := map[string]string{}
	for _, c := range TerminalCapabilities(env) {
		out[c.Terminfo] = c.Value
	}
	return out
}

// index is where a capability sits in its array, so that a fixture can be
// written in terms of names.
func index(t *testing.T, names []terminfoName, want string) int {
	t.Helper()
	for i, n := range names {
		if n.terminfo == want {
			return i
		}
	}
	t.Fatalf("no capability named %q in the name table", want)
	return -1
}

// The capability #2076 is about, with the bytes real zsh answers with.
//
// `\e[A` — `1b 5b 41` — is `xterm-256color`'s `cuu1`, and it is asserted as
// the three bytes rather than as an escape because the failure this closes
// was a *substitution*: powerlevel10k tests `$+terminfo[cuu1]` and builds a
// different prompt when the answer is no, so a plausible value would be worse
// than none. A wrong byte here is a prompt that draws in the wrong place with
// the theme's own have-I-got-it test satisfied.
func TestTheCursorUpCapabilityIsTheTerminalsOwnBytes(t *testing.T) {
	up := index(t, terminfoStringNames, "cuu1")
	caps := capabilitiesByName(database(t, "fixtureterm", terminfofixture.Description{
		Name: "fixtureterm", StrCount: up + 1,
		Strs: map[int]string{up: "\x1b[A"},
	}))
	if got, ok := caps["cuu1"]; !ok || got != "\x1b[A" {
		t.Errorf("terminfo[cuu1] = %q (present %v), want %q", got, ok, "\x1b[A")
	}
}

// Every boolean name answers, whether the description stores it or not.
//
// Measured against zsh 5.9.2: `$terminfo` under `TERM=dumb` is 50 keys and 44
// of them are booleans, though `dumb`'s description stores far fewer. A
// reader that answered only the stored ones would leave `$+terminfo[bce]` at
// 0 for most terminals, which is the same silent-substitution failure #2076
// is about with a different key.
func TestEveryBooleanNameAnswersPastTheStoredCount(t *testing.T) {
	on := index(t, terminfoBooleanNames, "am")
	bools := make([]byte, on+1)
	bools[on] = 1
	caps := capabilitiesByName(database(t, "boolterm",
		terminfofixture.Description{Name: "boolterm", Bools: bools}))
	for _, name := range terminfoBooleanNames {
		got, ok := caps[name.terminfo]
		if !ok {
			t.Errorf("boolean %q is not answered, and every boolean name is", name.terminfo)
			continue
		}
		want := "no"
		if name.terminfo == "am" {
			want = "yes"
		}
		if got != want {
			t.Errorf("terminfo[%s] = %q, want %q", name.terminfo, got, want)
		}
	}
}

// A numeric or string capability the description does not carry is not a key.
//
// Absent rather than empty, which is the distinction #1388 turned on: a
// caller reading an empty string cannot tell a terminal without the
// capability from a shell that never knew it, and `${terminfo[x]-default}` is
// how a theme asks.
func TestAnAbsentNumberOrStringIsNotAKey(t *testing.T) {
	colors := index(t, terminfoNumberNames, "colors")
	lines := index(t, terminfoNumberNames, "lines")
	up := index(t, terminfoStringNames, "cuu1")
	down := index(t, terminfoStringNames, "cud1")
	nums := make([]int, max(colors, lines)+1)
	for i := range nums {
		nums[i] = terminfofixture.Absent
	}
	nums[colors] = 256
	caps := capabilitiesByName(database(t, "sparseterm", terminfofixture.Description{
		Name: "sparseterm", Nums: nums, StrCount: max(up, down) + 1,
		Strs: map[int]string{up: "\x1b[A"},
	}))
	if got, ok := caps["colors"]; !ok || got != "256" {
		t.Errorf("terminfo[colors] = %q (present %v), want 256", got, ok)
	}
	for _, name := range []string{"lines", "cud1"} {
		if got, ok := caps[name]; ok {
			t.Errorf("terminfo[%s] = %q and the description does not carry it; it has to be absent", name, got)
		}
	}
}

// A numeric capability set to zero is carried, and only a negative one is
// absent.
//
// The off-by-one this catches is real: `ncv` is legitimately 0 in
// descriptions where color and video attributes do not interfere, and a
// reader treating falsy as absent would drop it.
func TestAZeroIsANumberAndNotAnAbsence(t *testing.T) {
	ncv := index(t, terminfoNumberNames, "ncv")
	nums := make([]int, ncv+1)
	for i := range nums {
		nums[i] = terminfofixture.Absent
	}
	nums[ncv] = 0
	caps := capabilitiesByName(database(t, "zeroterm",
		terminfofixture.Description{Name: "zeroterm", Nums: nums}))
	if got, ok := caps["ncv"]; !ok || got != "0" {
		t.Errorf("terminfo[ncv] = %q (present %v), want 0", got, ok)
	}
}

// The wider number format reads, and a count no 16-bit field could hold comes
// back whole.
//
// Both formats are on disk — a database built by a current `tic` holds a
// mixture — so a reader that knew one would answer nothing for descriptions
// written in the other, which looks exactly like a terminal it has never
// heard of.
func TestTheWideNumberFormatIsRead(t *testing.T) {
	pairs := index(t, terminfoNumberNames, "pairs")
	nums := make([]int, pairs+1)
	for i := range nums {
		nums[i] = terminfofixture.Absent
	}
	nums[pairs] = 65536
	caps := capabilitiesByName(database(t, "wideterm",
		terminfofixture.Description{Name: "wideterm", Wide: true, Nums: nums}))
	if got, ok := caps["pairs"]; !ok || got != "65536" {
		t.Errorf("terminfo[pairs] = %q (present %v), want 65536", got, ok)
	}
}

// The extended section answers under the names it carries, with its absent
// entries absent.
//
// This is where a modern description keeps the capabilities the standard set
// has no slot for, and a prompt reading `$terminfo[Se]` to restore the cursor
// shape is reading one of them.
func TestTheExtendedSectionIsRead(t *testing.T) {
	caps := capabilitiesByName(database(t, "extterm", terminfofixture.Description{
		Name: "extterm",
		Ext: &terminfofixture.Extended{
			Bools:  []byte{1},
			Nums:   []int{16},
			Values: []string{"\x1b[2 q", terminfofixture.AbsentString},
			Names:  []string{"AX", "U8", "Se", "Ss"},
		},
	}))
	for _, tc := range []struct{ name, want string }{
		{"AX", "yes"}, {"U8", "16"}, {"Se", "\x1b[2 q"},
	} {
		if got, ok := caps[tc.name]; !ok || got != tc.want {
			t.Errorf("terminfo[%s] = %q (present %v), want %q", tc.name, got, ok, tc.want)
		}
	}
	if got, ok := caps["Ss"]; ok {
		t.Errorf("terminfo[Ss] = %q and its value is stored absent; it has to be absent", got)
	}
}

// The two booleans that are read from a string rather than from their own
// slot.
//
// Measured against zsh 5.9.2 with descriptions compiled for the purpose, and
// it matters on real terminals in both directions: `ansi` stores `OTbs` and
// spells `cub1` as `\e[D`, while `linux`, `hpterm`, `sun`, `aixterm`,
// `cygwin` and `putty` leave the bit clear and spell `cub1` as a backspace.
func TestTheDerivedBooleansComeFromTheStringTheyDescribe(t *testing.T) {
	back := index(t, terminfoStringNames, "cub1")
	newline := index(t, terminfoStringNames, "nel")
	bs := index(t, terminfoBooleanNames, "OTbs")
	nl := index(t, terminfoBooleanNames, "OTNL")
	bits := make([]byte, max(bs, nl)+1)
	bits[bs], bits[nl] = 1, 1
	for _, tc := range []struct {
		name       string
		strs       map[int]string
		bs, nl     string
		storedOnly bool
	}{
		{name: "derived", strs: map[int]string{back: "\b", newline: "\n"}, bs: "yes", nl: "yes"},
		{name: "overruled", strs: map[int]string{back: "\x1b[D", newline: "\r\n"}, bs: "no", nl: "no"},
		{name: "nostrings", strs: map[int]string{}, bs: "yes", nl: "no"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := capabilitiesByName(database(t, tc.name, terminfofixture.Description{
				Name: tc.name, Bools: bits,
				StrCount: max(back, newline) + 1, Strs: tc.strs,
			}))
			if got := caps["OTbs"]; got != tc.bs {
				t.Errorf("terminfo[OTbs] = %q, want %q", got, tc.bs)
			}
			if got := caps["OTNL"]; got != tc.nl {
				t.Errorf("terminfo[OTNL] = %q, want %q", got, tc.nl)
			}
		})
	}
}

// A `$TERM` with no description answers nothing at all.
//
// Measured: zsh's `${+terminfo}` is 1 and `${#terminfo}` is 0 under a `$TERM`
// its database has never heard of. The parameter exists and the table is
// empty, which is what a script testing `$+terminfo[cuu1]` is written
// against.
func TestATermWithNoDescriptionAnswersNothing(t *testing.T) {
	dir := terminfofixture.Database(t, terminfofixture.Description{Name: "knownterm"})
	for _, term := range []string{"unknownterm", "", ".", "..", ".hidden", "a/b", `a\b`} {
		caps := TerminalCapabilities(environment(map[string]string{
			"TERM": term, "TERMINFO": dir,
		}))
		if len(caps) != 0 {
			t.Errorf("TERM=%q answered %d capabilities, want none", term, len(caps))
		}
	}
}

// A terminal name is one path component, so a database directory is the only
// thing a lookup can reach into.
//
// The discriminating case is a name that would resolve to a real file if it
// were joined rather than checked: `$TERM` is ordinary environment and a
// script can set it, and the read this package makes is exempt from the
// boundary on the argument that the shell chose the path. That argument is
// only true while the name cannot leave the directory.
func TestATerminalNameCannotLeaveTheDatabaseDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside")
	if err := os.WriteFile(outside, terminfofixture.Description{Name: "x"}.Bytes(), 0o644); err != nil {
		t.Fatalf("writing the file outside: %v", err)
	}
	inner := filepath.Join(dir, "db")
	if err := os.MkdirAll(filepath.Join(inner, "x"), 0o755); err != nil {
		t.Fatalf("making the database: %v", err)
	}
	for _, term := range []string{"../outside", "..", "./x"} {
		if terminalNameIsOneComponent(term) {
			t.Errorf("TERM=%q is treated as a terminal name, and it is a path", term)
		}
	}
}

// A database directory whose subdirectory is the first character's
// hexadecimal value is searched too, which is the layout macOS ships.
func TestTheHexadecimalDirectorySpellingIsFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "78"), 0o755); err != nil {
		t.Fatalf("making the database: %v", err)
	}
	up := index(t, terminfoStringNames, "cuu1")
	d := terminfofixture.Description{Name: "xhexterm", StrCount: up + 1, Strs: map[int]string{up: "\x1b[A"}}
	if err := os.WriteFile(filepath.Join(dir, "78", "xhexterm"), d.Bytes(), 0o644); err != nil {
		t.Fatalf("writing the description: %v", err)
	}
	caps := capabilitiesByName(environment(map[string]string{"TERM": "xhexterm", "TERMINFO": dir}))
	if got, ok := caps["cuu1"]; !ok || got != "\x1b[A" {
		t.Errorf("terminfo[cuu1] = %q (present %v) under the hexadecimal spelling, want %q", got, ok, "\x1b[A")
	}
}

// `$TERMINFO` is searched before the personal database, and the personal one
// before what `$TERMINFO_DIRS` names.
//
// Order is the whole of what a search path is, and it is checkable without
// touching the system directories: three databases, the same terminal in each
// with a different value, and which value comes back says which was reached
// first.
func TestTheSearchPathIsInOrder(t *testing.T) {
	up := index(t, terminfoStringNames, "cuu1")
	write := func(where, value string) string {
		dir := filepath.Join(t.TempDir(), where)
		if err := os.MkdirAll(filepath.Join(dir, "o"), 0o755); err != nil {
			t.Fatalf("making %s: %v", where, err)
		}
		d := terminfofixture.Description{Name: "orderterm", StrCount: up + 1, Strs: map[int]string{up: value}}
		if err := os.WriteFile(filepath.Join(dir, "o", "orderterm"), d.Bytes(), 0o644); err != nil {
			t.Fatalf("writing into %s: %v", where, err)
		}
		return dir
	}
	own, home, listed := write("own", "own"), write("home", "home"), write("listed", "listed")
	vars := map[string]string{
		"TERM": "orderterm", "TERMINFO": own,
		"HOME": filepath.Dir(home), "TERMINFO_DIRS": listed,
	}
	// The personal database is `$HOME/.terminfo`, so the home fixture has to
	// be reachable under that name.
	if err := os.Symlink(home, filepath.Join(filepath.Dir(home), ".terminfo")); err != nil {
		t.Fatalf("linking the personal database: %v", err)
	}
	for _, tc := range []struct{ drop, want string }{
		{drop: "", want: "own"},
		{drop: "TERMINFO", want: "home"},
		{drop: "HOME", want: "listed"},
	} {
		if tc.drop != "" {
			delete(vars, tc.drop)
		}
		if got := capabilitiesByName(environment(vars))["cuu1"]; got != tc.want {
			t.Errorf("with %v, terminfo[cuu1] = %q, want %q", vars, got, tc.want)
		}
	}
}

// A file that is not a description reads as no description, whatever is wrong
// with it.
//
// Every failure is the same answer because a caller can do nothing different
// with a truncated file than with a missing one — and because the read is of
// a path assembled from `$TERM`, so a file that happens to be there and
// happens not to parse must not be a diagnostic about what is on disk.
func TestAFileThatIsNotADescriptionAnswersNothing(t *testing.T) {
	good := terminfofixture.Description{Name: "goodterm", Nums: []int{80}}.Bytes()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "b"), 0o755); err != nil {
		t.Fatalf("making the database: %v", err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "truncated header", data: good[:8]},
		{name: "truncated body", data: good[:len(good)-4]},
		{name: "foreign magic", data: append([]byte{0x7f, 'E', 'L', 'F'}, good[4:]...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(dir, "b", "badterm"), tc.data, 0o644); err != nil {
				t.Fatalf("writing: %v", err)
			}
			if caps := TerminalCapabilities(environment(map[string]string{
				"TERM": "badterm", "TERMINFO": dir,
			})); len(caps) != 0 {
				t.Errorf("a %s file answered %d capabilities, want none", tc.name, len(caps))
			}
		})
	}
}

// Nothing is read when there is no environment to read it from.
//
// An embedder holding a Runner with no `$TERM` is the ordinary case, not an
// error, and this is the guard that keeps it from becoming one.
func TestNoEnvironmentIsNoDescription(t *testing.T) {
	if caps := TerminalCapabilities(nil); len(caps) != 0 {
		t.Errorf("a nil environment answered %d capabilities, want none", len(caps))
	}
}

// The name tables are the three arrays' index orders, and each name appears
// once.
//
// What this catches is the edit the tables invite: a capability inserted in
// the middle to keep the file alphabetical, which silently renames every slot
// after it — `cuu1` would then answer with its neighbour's bytes and nothing
// would fail. The counts are the measurement in terminfonames.go.
func TestTheNameTablesAreWholeAndUnique(t *testing.T) {
	for _, tc := range []struct {
		what  string
		names []terminfoName
		count int
	}{
		{what: "booleans", names: terminfoBooleanNames, count: 44},
		{what: "numbers", names: terminfoNumberNames, count: 39},
		{what: "strings", names: terminfoStringNames, count: 413},
	} {
		if len(tc.names) != tc.count {
			t.Errorf("the %s table holds %d names and a description stores %d slots", tc.what, len(tc.names), tc.count)
		}
	}
	seen := map[string]string{}
	for _, tc := range []struct {
		what  string
		names []terminfoName
	}{
		{what: "boolean", names: terminfoBooleanNames},
		{what: "number", names: terminfoNumberNames},
		{what: "string", names: terminfoStringNames},
	} {
		for i, n := range tc.names {
			if n.terminfo == "" {
				continue
			}
			if where, ok := seen[n.terminfo]; ok {
				t.Errorf("%q is the %s at index %d and also %s", n.terminfo, tc.what, i, where)
			}
			seen[n.terminfo] = tc.what
			if n.termcap == "" {
				t.Errorf("%s %q has no termcap code, so $termcap cannot answer for it", tc.what, n.terminfo)
			}
		}
	}
	// The one place the tables are pinned against something outside
	// themselves: three capabilities whose position decides whether every
	// later slot is read under the right name.
	for _, tc := range []struct {
		names []terminfoName
		name  string
		at    int
	}{
		{names: terminfoBooleanNames, name: "hc", at: 7},
		{names: terminfoNumberNames, name: "colors", at: 13},
		{names: terminfoStringNames, name: "cuu1", at: 19},
	} {
		if got := index(t, tc.names, tc.name); got != tc.at {
			t.Errorf("%q is at index %d and a compiled description stores it at %d", tc.name, got, tc.at)
		}
	}
}
