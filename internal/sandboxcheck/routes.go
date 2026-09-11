// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package sandboxcheck

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// A Route is one way a script can reach outside itself, as a script.
//
// The table below is the instrument. It is written as *routes a script can
// take* rather than as rules a policy can express, because the two are not
// the same list and the gap between them is where every escape here has
// lived: the policy format could always say `deny write`, and `zf_rm` went
// on deleting things regardless.
type Route struct {
	// Name is how the row prints. Kept short and grep-able, because -run
	// matches on it.
	Name string
	// Only restricts the route to dialects that have it. Empty means all
	// four — and the default is deliberate: a route nobody marked is run
	// everywhere, so a builtin that quietly appears in a second dialect is
	// graded there too.
	Only []string
	// Script is the whole of the attempt, with the fixture's paths
	// substituted. It runs from inside the workspace.
	Script string
	// Did reports whether the attempt worked, and is the only thing that
	// decides a verdict. Most ask the filesystem; the ones that cannot —
	// an exec, a signal — ask what the script managed to print.
	Did func(f Fixture, o Outcome) bool
	// Why says what the route is, for a row that needs explaining.
	Why string
}

func (rt Route) dialects() []string {
	if len(rt.Only) == 0 {
		return AllDialects
	}
	return rt.Only
}

func (rt Route) script(f Fixture) string {
	rep := strings.NewReplacer(
		"{{secret}}", f.Secret,
		"{{target}}", f.Target,
		"{{victim}}", f.Victim,
		"{{victimdir}}", f.VictimDir,
		"{{link}}", f.Link,
		"{{sock}}", f.Sock,
		"{{outside}}", f.Root,
		"{{ws}}", f.Ws,
		"{{carved}}", f.Carved,
		"{{carvedupper}}", f.CarvedUpper,
		"{{carveddirupper}}", f.CarvedDirUpper,
	)
	return rep.Replace(rt.Script)
}

func asExit(err error, into **exec.ExitError) bool { return errors.As(err, into) }

// gone reports that a path is no longer there, which is how a removal route
// says it worked.
func gone(path string) bool {
	_, err := os.Lstat(path)
	return err != nil
}

func there(path string) bool { return !gone(path) }

// leaked reports that the contents of the secret reached the script.
func leaked(_ Fixture, o Outcome) bool { return o.Says(SecretMark) }

// made reports that the route created the target.
func made(f Fixture, _ Outcome) bool { return there(f.Target) }

// Routes is every way in, in the order a reader wants them: the ordinary
// language first, then the places a builtin reaches past it.
func Routes() []Route {
	return append(coreRoutes(), moduleRoutes()...)
}

// coreRoutes are the shell language itself — redirection, expansion,
// sourcing, the things every dialect has. These are the routes the gate was
// built for, and they are here because a fix elsewhere that broke one of them
// should be loud.
func coreRoutes() []Route {
	return append([]Route{{
		Name:   "write/redirect",
		Script: `echo x > {{target}}`,
		Did:    made,
		Why:    "the plainest write there is",
	}, {
		Name:   "write/append",
		Script: `echo x >> {{target}}`,
		Did:    made,
		Why:    "appending creates the file too",
	}, {
		Name:   "write/clobber",
		Script: `echo x >| {{target}}`,
		Did:    made,
		Why:    "the form that overrides noclobber",
	}, {
		Name:   "write/exec-fd",
		Script: `exec 3> {{target}}; echo x >&3; exec 3>&-`,
		Did:    made,
		Why:    "a descriptor opened once and written later",
	}, {
		Name:   "write/readwrite-fd",
		Script: `exec 3<> {{target}}; exec 3>&-`,
		Did:    made,
		Why:    "opening for both creates the file",
	}, {
		Name:   "write/heredoc",
		Script: "while read l; do echo $l; done <<EOF > {{target}}\nx\nEOF",
		Did:    made,
		Why:    "a here-document's output still has to land somewhere",
	}, {
		Name:   "write/relative-parent",
		Script: `echo x > ../target`,
		Did:    made,
		Why:    "the workspace's own parent, reached by name rather than by path",
	}, {
		Name:   "write/truncate-victim",
		Script: `: > {{victim}}`,
		Did:    func(f Fixture, _ Outcome) bool { return size(f.Victim) == 0 },
		Why:    "destroying a file without writing a byte to it",
	}, {
		Name:   "write/through-symlink",
		Script: `echo x > {{link}}/target`,
		Did:    made,
		Why:    "a name the policy allows, reaching an object it does not",
	}, {
		Name:   "write/truncate-through-symlink",
		Script: `: > {{link}}/victim`,
		Did:    func(f Fixture, _ Outcome) bool { return size(f.Victim) == 0 },
		Why:    "the same, destroying a file rather than making one",
	}, {
		Name:   "read/redirect",
		Script: `read L < {{secret}}; echo $L`,
		Did:    leaked,
		Why:    "the plainest read there is",
	}, {
		Name:   "read/substitution",
		Script: `echo $(< {{secret}})`,
		Did:    leaked,
		Only:   []string{"bash", "zsh", "ksh"},
		Why:    "the form that reads a file without running a command",
	}, {
		Name:   "read/source",
		Script: `echo 'X=' > /dev/null; . {{secret}}`,
		Did:    leaked,
		Why:    "sourcing runs the file, so a refusal here is the one that matters most",
	}, {
		Name:   "read/exec-fd",
		Script: `exec 3< {{secret}}; read L <&3; echo $L`,
		Did:    leaked,
		Why:    "a descriptor opened for reading and used later",
	}, {
		Name:   "read/through-symlink",
		Script: `read L < {{link}}/secret; echo $L`,
		Did:    leaked,
		Why:    "the read half of a name that resolves out of the workspace",
	}, {
		Name:   "probe/test",
		Script: `if [ -f {{secret}} ]; then echo SEEN; fi`,
		Did:    func(_ Fixture, o Outcome) bool { return o.Says("SEEN") },
		Why:    "a probe is an oracle: existence is information",
	}, {
		Name:   "list/glob",
		Script: `echo {{outside}}/*`,
		Did:    func(_ Fixture, o Outcome) bool { return o.Says("secret") },
		Why:    "a glob enumerates a directory it was never allowed to read",
	}, {
		Name:   "exec/external",
		Script: `/bin/echo RAN`,
		Did:    func(_ Fixture, o Outcome) bool { return o.Says("RAN") },
		Why:    "an allowed exec is the whole filesystem, so a denied one has to hold",
	}, {
		Name:   "signal/kill",
		Script: `kill -0 $$ && echo SIGNALED`,
		Did:    func(_ Fixture, o Outcome) bool { return o.Says("SIGNALED") },
		Why:    "the one action with no path, which no path rule can refuse",
	}}, spellingRoutes()...)
}

// spellingRoutes are the same file under the other names its volume answers
// to — #2044.
//
// These are the only routes here that aim *inside* the workspace, and they
// are the reason Fixture.Carved exists. A rule matches a name; a filesystem
// may hold one object under a whole class of names, and on macOS's default
// volumes — and on Windows, and on a case-insensitive mount anywhere —
// `.env` and `.ENV` are one file. A deny covering one spelling covered the
// object only by luck, and the measured consequence was a policy of exactly
// the shape below handing over a live credential and then overwriting it.
//
// # Why these grade INERT on a case-sensitive volume, and why that is right
//
// The ungated run has to reach the file for the row to mean anything. Where
// the volume is case-sensitive there is no second name — `SECRET.TXT` is not
// there at all — so the ungated read finds nothing, and verdictOf calls that
// inert rather than contained. That is the instrument declining to credit a
// gate with stopping something the filesystem was never going to do, which is
// the rule the whole table is shaped around, and it means one sweep reads
// honestly on both platforms without either being told which it is.
//
// # What is deliberately not here
//
// The composition half — a name stored `caf\u00e9` and reached as
// `cafe\u0301`. It is a live escape, it is #2045, and a route for it would
// turn `make sandbox` red on `main` while the sweep gates CI. It is filed
// rather than staged.
func spellingRoutes() []Route {
	return []Route{{
		Name:   "spelling/read-upper-leaf",
		Script: `read -r x < {{carvedupper}} && echo "$x"`,
		Did:    leaked,
		Why:    "the denied file under a respelled leaf, which is the same file",
	}, {
		Name:   "spelling/read-upper-dir",
		Script: `read -r x < {{carveddirupper}} && echo "$x"`,
		Did:    leaked,
		Why:    "a respelled *directory* component, because a rule is about a path",
	}, {
		Name:   "spelling/write-upper-leaf",
		Script: `echo overwritten > {{carvedupper}}`,
		Did:    overwritten,
		Why:    "the damaging half: a denied file rewritten through its other name",
	}, {
		Name:   "spelling/truncate-upper-leaf",
		Script: `: > {{carvedupper}}`,
		Did:    overwritten,
		Why:    "O_TRUNC empties as part of the open, so a late refusal is too late",
	}}
}

// overwritten reports that the carved-out file no longer holds what the
// fixture put in it, which is how a write route against it says it worked.
//
// Read back from the file rather than from the script's output, because a
// write route that was refused can still exit zero — the redirection fails,
// the builtin never runs, and a verdict taken from the exit status would
// call that containment. The filesystem is the only witness that cannot be
// talked out of it.
func overwritten(f Fixture, _ Outcome) bool {
	b, err := os.ReadFile(f.Carved)
	if err != nil {
		// Unreadable is not "unchanged": a route that removed or replaced
		// the file got past the gate just as surely as one that rewrote it.
		return true
	}
	return !strings.Contains(string(b), SecretMark)
}

// moduleRoutes are the builtins that reach the filesystem past the language.
//
// Every escape this repository has had is in this list, which is the argument
// for the list existing: each was reachable because the code implementing it
// had no reason to know a gate was there, and none of them is expressible as
// a redirection.
func moduleRoutes() []Route {
	zsh := []string{"zsh"}
	return []Route{{
		Name:   "module/sysopen-read",
		Only:   zsh,
		Script: `zmodload zsh/system; sysopen -r -u 7 {{secret}} && sysread -i 7 x && echo $x`,
		Did:    leaked,
		Why:    "#1805: opened any file with a bare os.OpenFile",
	}, {
		Name:   "module/sysopen-create",
		Only:   zsh,
		Script: `zmodload zsh/system; sysopen -o creat -w -u 7 {{target}} && syswrite -o 7 x`,
		Did:    made,
		Why:    "#1805: creating is a write",
	}, {
		Name: "module/flock",
		Only: zsh,
		// Against a file that is already there, because taking a lock does
		// not create one — aimed at a name that does not exist the route
		// fails for that reason and grades as inert, which would hide it.
		Script: `zmodload zsh/system; zsystem flock -t 1 {{victim}} && echo LOCKED`,
		Did:    func(_ Fixture, o Outcome) bool { return o.Says("LOCKED") },
		Why:    "#1805: a lock is taken by opening the file",
	}, {
		Name:   "module/autoload",
		Only:   zsh,
		Script: `print 'echo ` + SecretMark + `' > {{outside}}/fn; fpath=({{outside}}); autoload -Uz fn; fn`,
		Did:    leaked,
		Why:    "#1812: autoload read a file with os.ReadFile and then ran it",
	}, {
		Name:   "module/zf_rm",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_rm {{victim}}`,
		Did:    func(f Fixture, _ Outcome) bool { return gone(f.Victim) },
		Why:    "#1819: deleted a denied file and returned 0",
	}, {
		Name:   "module/zf_rm-r",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_rm -r {{victimdir}}`,
		Did:    func(f Fixture, _ Outcome) bool { return gone(f.VictimDir) },
		Why:    "#1819: and a whole tree with it",
	}, {
		Name:   "module/zf_rmdir",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_rmdir {{victimdir}}/entry`,
		Did:    func(f Fixture, _ Outcome) bool { return gone(f.VictimDir + "/entry") },
		Why:    "#1819",
	}, {
		Name:   "module/zf_mkdir",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_mkdir {{target}}`,
		Did:    made,
		Why:    "#1819",
	}, {
		Name:   "module/zf_mkdir-p",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_mkdir -p {{target}}/a/b`,
		Did:    made,
		Why:    "#1819: -p creates parents nobody named",
	}, {
		Name:   "module/zf_mv",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_mv {{victim}} {{target}}`,
		Did:    made,
		Why:    "#1819",
	}, {
		Name:   "module/zf_ln-hard",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_ln {{secret}} ./leak && read L < ./leak && echo $L`,
		Did:    leaked,
		Why:    "#1819: a hard link is a second name, so the contents arrive inside",
	}, {
		Name:   "module/zf_chmod",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_chmod 777 {{victim}}`,
		Did:    func(f Fixture, _ Outcome) bool { return perm(f.Victim) == 0o777 },
		Why:    "#1819",
	}, {
		Name: "module/zf_chmod-link",
		Only: zsh,
		Script: `zmodload zsh/files; zf_ln -s {{victim}} ./sneaky
zf_chmod 777 ./sneaky`,
		Did: func(f Fixture, _ Outcome) bool { return perm(f.Victim) == 0o777 },
		Why: "#1819: an allowed name and a denied object",
	}, {
		Name:   "module/zstat",
		Only:   zsh,
		Script: `zmodload zsh/stat; zstat -A s +size {{secret}}; echo SIZE=$s`,
		Did: func(_ Fixture, o Outcome) bool {
			return o.Says("SIZE=" + strconv.Itoa(len(SecretMark)+1))
		},
		Why: "#1819: answered with the true size of a file `[ -f ]` hides",
	}, {
		Name:   "module/zsocket",
		Only:   zsh,
		Script: `zmodload zsh/net/socket; zsocket -l {{sock}}`,
		Did:    func(f Fixture, _ Outcome) bool { return there(f.Sock) },
		Why:    "#1819: binding leaves a socket on the filesystem",
	}, {
		// Not implemented today, which is the point of grading it: this is
		// the ledger #1808 asks for. The day mapfile lands, this row stops
		// being inert and says whether it arrived inside the boundary.
		Name:   "module/mapfile-read",
		Only:   zsh,
		Script: `zmodload zsh/mapfile; echo ${mapfile[{{secret}}]}`,
		Did:    leaked,
		Why:    "not implemented yet — will need a gate when it is",
	}, {
		Name:   "module/mapfile-write",
		Only:   zsh,
		Script: `zmodload zsh/mapfile; mapfile[{{target}}]=x`,
		Did:    made,
		Why:    "not implemented yet — will need a gate when it is",
	}, {
		Name:   "builtin/history-write",
		Only:   []string{"bash"},
		Script: `history -w {{target}}`,
		Did:    made,
		Why:    "not a builtin yet — falls through to a refused exec",
	}, {
		Name:   "builtin/mapfile",
		Only:   []string{"bash"},
		Script: `mapfile -t A < {{secret}}; echo $A`,
		Did:    leaked,
		Why:    "reads through a redirection, so it answers to the redirection's gate",
	}, {
		Name:   "builtin/fc-write",
		Only:   zsh,
		Script: `HISTFILE={{target}}; SAVEHIST=10; fc -W`,
		Did:    made,
		Why:    "-W is not accepted yet — a history file is a write to a path the script names",
	}, {
		Name:   "builtin/zcompile",
		Only:   zsh,
		Script: `echo ':' > c.zsh; zcompile {{target}} c.zsh`,
		Did:    made,
		Why:    "#1405: it writes a compiled file wherever it is told to",
	}}
}

func size(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

func perm(path string) os.FileMode {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	return info.Mode().Perm()
}
