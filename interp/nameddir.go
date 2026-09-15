// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"strings"
)

// The table behind `~name`, which is a directory this shell was told about
// rather than one the operating system knows.
//
// Two things wear a `~` and a name, and only one of them is the shell's.
// `~user` is the user database's answer and is reached through
// Runner.UserHomeDir, because the core may not ask the process a question the
// Runner should hold — the rule interp/lookpath.go records. `~name` is a
// *named directory*: a table this shell owns outright, written by `hash -d`
// and read here, which needs nothing from the operating system at all.
//
// Measured on zsh 5.9.2, 2026-09-14, and the order matters: a named directory
// **wins** over a user of the same name. `hash -d root=/tmp; print -r -- ~root`
// is `/tmp` where the same line without the assignment is `/var/root`.

// namedDir reads one entry, or reports that there is none.
func (r *Runner) namedDir(name string) (string, bool) {
	dir, ok := r.namedDirs[name]
	return dir, ok
}

// putNamedDir writes one, replacing whatever was there: measured, a second
// `hash -d a=…` is the value that stands.
func (r *Runner) putNamedDir(name, dir string) {
	if r.namedDirs == nil {
		r.namedDirs = make(map[string]string)
	}
	r.namedDirs[name] = dir
}

// namedDirNames is the table's names in the order a listing writes them,
// which is sorted rather than the order they were written: measured,
// `hash -d z=/z b=/b a=/a; hash -d` lists `a b z`.
func (r *Runner) namedDirNames() []string {
	names := make([]string, 0, len(r.namedDirs))
	for name := range r.namedDirs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// validNamedDirName reports whether a name may stand in the table. A `/` is
// what the one shell with this construct refuses, and it has to: the name a
// `~` reads runs to the first slash, so an entry holding one could never be
// read back.
func validNamedDirName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/=")
}

// namedDirValue is one entry's value as a listing writes it: quoted where it
// would not read back as itself, which is what makes an empty value visible
// as `”` rather than as nothing at all.
func namedDirValue(v string) string {
	if v != "" && strings.IndexFunc(v, needsNamedDirQuote) < 0 {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// needsNamedDirQuote is deliberately conservative — a character it is unsure
// about is quoted — because a listing that quotes too much still reads back
// and one that quotes too little does not.
func needsNamedDirQuote(c rune) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return false
	}
	return !strings.ContainsRune("_-./:+@%,", c)
}
