// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// How a prompt shortens a directory, asked of the substrate rather than of a
// shell: against the home directory, against the table a `hash -d` fills, and
// against the counting that reads whatever came out.
//
// A dialect's answers live in its own package. What is here is what the
// substrate promises whoever fills those in.

// TestAValueEndingInASeparatorAbbreviatesNothing is #4539.
//
// Measured 2026-09-26 against zsh 5.9.2 and bash 5.3.20, which agree on every
// row. The last two rows are the control, and they are why this is a rule
// about the trailing separator rather than a guard against the root: the
// helper is right for an ordinary value, so what the rest says is that the
// separator switches abbreviation off.
//
// Written as a prefix test alone, every absolute path on the machine has `/`
// in front of it, so a home of `/` abbreviated all of them — `~usr` for
// `/usr`, and `~` for `/` itself, neither of which any reference writes.
func TestAValueEndingInASeparatorAbbreviatesNothing(t *testing.T) {
	for _, tc := range []struct{ dir, against, want string }{
		{"/usr/local", "/", "/usr/local"},
		{"/", "/", "/"},
		{"/usr", "//", "/usr"},
		{"/usr/local", "/usr/", "/usr/local"},
		{"/usr", "/usr/", "/usr"},
		// The controls: an ordinary value still abbreviates, exactly and
		// below.
		{"/usr/local", "/usr", "~/local"},
		{"/usr", "/usr", "~"},
		// And a value that is merely a prefix of the letters is not a
		// directory the path is under.
		{"/usrlocal", "/usr", "/usrlocal"},
		{"/usr", "", "/usr"},
		{"", "/usr", ""},
	} {
		if got := abbreviateHome(tc.dir, tc.against); got != tc.want {
			t.Errorf("abbreviateHome(%q, %q) = %q, want %q", tc.dir, tc.against, got, tc.want)
		}
	}
}

// TestTheShortestDrawnCandidateWins is #4541: a named directory is one of the
// candidates a prompt shortens against, and the one that draws the shortest
// string is the one it uses.
//
// Measured on zsh 5.9.2, 2026-09-26. The discriminating rows are the two where
// a *longer* match loses — `longname` and `xxxxx` each cover more of the path
// than the candidate that wins and are written under a longer marker — so a
// reading of "the longest matching value" passes every other row here and
// fails those two.
func TestTheShortestDrawnCandidateWins(t *testing.T) {
	const nd = "/n/d"
	for _, tc := range []struct {
		name  string
		home  string
		names map[string]string
		dir   string
		want  string
	}{
		{"no table and no home", "", nil, nd + "/a/b", nd + "/a/b"},
		{"the named directory itself", "", map[string]string{"jd": nd}, nd, "~jd"},
		{"below it", "", map[string]string{"jd": nd}, nd + "/a/b", "~jd/a/b"},
		{"the longer match, written shorter", "", map[string]string{"jd": nd, "ab": nd + "/a"}, nd + "/a/b", "~ab/b"},
		{"the longer match, written longer", "", map[string]string{"xxxxx": nd + "/a", "y": nd}, nd + "/a/b", "~y/a/b"},
		{"a name beating the home", nd, map[string]string{"x": nd + "/a"}, nd + "/a/b", "~x/b"},
		{"a name losing to the home", nd, map[string]string{"longname": nd + "/a"}, nd + "/a/b", "~/a/b"},
		{"a tie goes to the home", nd, map[string]string{"ab": nd + "/a"}, nd + "/a/b", "~/a/b"},
		{"a tie between names goes to the first by name", "", map[string]string{"z1": nd, "a1": nd}, nd + "/a", "~a1/a"},
		{"a value ending in a separator is no candidate", "", map[string]string{"jd": "/"}, "/usr", "/usr"},
		{"nothing over it", "", map[string]string{"jd": "/elsewhere"}, nd + "/a", nd + "/a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Runner{namedDirs: tc.names}
			if tc.home != "" {
				r.setVar("HOME", tc.home)
			}
			if got := r.abbreviatedDirectory(tc.dir); got != tc.want {
				t.Errorf("abbreviatedDirectory(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

// TestANamedDirectorysWholeMarkerIsOneUnit is the other half of #4541: the
// count in front of a directory code treats `~jd` as one thing, exactly as it
// treats `~`.
//
// Measured on zsh 5.9.2, 2026-09-26, in `~jd/a/b` — `%3~` is the whole path
// with the marker back and `%-1~` is the marker alone, which are the same two
// answers `~/a/b` gives. Reading only the tilde made `%-1~` draw `~` and `%c`
// at a named root draw `jd`, neither of which names a directory.
func TestANamedDirectorysWholeMarkerIsOneUnit(t *testing.T) {
	for _, tc := range []struct {
		path       string
		lead, rest string
	}{
		{"~jd/a/b", "~jd", "a/b"},
		{"~jd", "~jd", ""},
		{"~/a/b", "~", "a/b"},
		{"~", "~", ""},
		{"/a/b", "/", "a/b"},
		{"a/b", "", "a/b"},
	} {
		if lead, rest := pathLeader(tc.path); lead != tc.lead || rest != tc.rest {
			t.Errorf("pathLeader(%q) = %q, %q; want %q, %q", tc.path, lead, rest, tc.lead, tc.rest)
		}
	}
	for _, tc := range []struct {
		path string
		n    int
		last string
		lead string
	}{
		// n at or past the units is the whole path, marker included — which
		// for `~jd/a/b` is three units and not four.
		{"~jd/a/b", 1, "b", "~jd"},
		{"~jd/a/b", 2, "a/b", "~jd/a"},
		{"~jd/a/b", 3, "~jd/a/b", "~jd/a/b"},
		{"~jd/a/b", 4, "~jd/a/b", "~jd/a/b"},
		// The control: a home marker counts the same way.
		{"~/a/b", 1, "b", "~"},
		{"~/a/b", 3, "~/a/b", "~/a/b"},
	} {
		if got := trailingComponents(tc.path, tc.n); got != tc.last {
			t.Errorf("trailingComponents(%q, %d) = %q, want %q", tc.path, tc.n, got, tc.last)
		}
		if got := leadingComponents(tc.path, tc.n); got != tc.lead {
			t.Errorf("leadingComponents(%q, %d) = %q, want %q", tc.path, tc.n, got, tc.lead)
		}
	}
}

// TestAPromptReadsItsDirectoryFromWhereTheStyleSaysItDoes is #4540, asked of
// the substrate: the two readings are both reachable and the style is what
// chooses between them.
//
// Which dialect holds which is measured in the dialect packages — see
// PromptStyle.CwdIsTheShellsOwnDirectory for the panel.
func TestAPromptReadsItsDirectoryFromWhereTheStyleSaysItDoes(t *testing.T) {
	r := &Runner{Dir: "/where/the/shell/is"}
	r.setVar("PWD", "/what/the/parameter/says")
	if got := r.promptCwd(PromptStyle{}); got != "/what/the/parameter/says" {
		t.Errorf("with the zero value the prompt read %q, want the parameter", got)
	}
	st := PromptStyle{CwdIsTheShellsOwnDirectory: true}
	if got := r.promptCwd(st); got != "/where/the/shell/is" {
		t.Errorf("with the field set the prompt read %q, want the shell's own directory", got)
	}
}
