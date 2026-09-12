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
	// Args is what the shell needs on its command line *before* `-c` for
	// this route to be reachable at all.
	//
	// Empty for every route but one, and the exception is the reason the
	// field exists. zsh writes a history file only when the shell is
	// interactive — measured, and not because the list is empty: a list
	// loaded by `fc -R` and printed by `fc -l` is still not written from a
	// plain `-c` run. So `builtin/fc-write` graded `inert` forever, and the
	// note beside it said the letter was not accepted, which was true and
	// was not why (#2283).
	//
	// A route that needs this is saying something worth seeing in the table:
	// the *shell's own mode* is part of what makes the route a route. It is
	// not a way to pass a policy — the mode and the policy are separate
	// arguments and the grader owns the second.
	Args []string
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
		"{{carvedaccentnfd}}", f.CarvedAccentNFD,
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
		Name:   "write/multios",
		Only:   []string{"zsh"},
		Script: `echo x > inside > {{target}}`,
		Did:    made,
		Why:    "one operator opening two files, so a gate that checked the first is past",
	}, {
		// The body of a `>(…)` runs beside the command that named it and
		// nothing waits for it — not the command, not the shell on its way
		// out, and not `wait`, which is true of the real shells too. So the
		// body has to hand the shell a rendezvous of its own, and the row is
		// only worth having if that rendezvous is *structural* rather than a
		// race this platform happens to win.
		//
		// Two spellings that looked synchronous are not, and both were caught
		// by giving the body a deliberate delay rather than by running it
		// often: reading the body's output through an enclosing `<(…)` returns
		// on end-of-file when the *outer* body exits, which does not wait for
		// the inner one, and a command substitution around the whole thing
		// ends with the command rather than with the body. Each of them
		// graded this row green on macOS and then inert on one dialect and
		// overblocked on two others on Linux — a gate that was working
		// perfectly, reported as broken.
		//
		// What works is the body saying so itself, in the workspace, *after*
		// the open it is being graded on. Every policy here permits the
		// workspace, so the mark arrives in all three runs — including the
		// denied one, where the open it follows was refused — and the shell
		// is looking at a settled filesystem either way. The bound on the
		// wait is a safety valve and nothing more: the mark lands in
		// milliseconds, and a sweep that hangs is worse than one that is
		// wrong out loud.
		Name: "write/procsub",
		Only: []string{"bash", "zsh", "ksh"},
		Script: `echo x > >(echo written > {{target}}; echo done > sync)
w=0; while [ ! -e sync ] && [ $w -lt 2000000 ]; do w=$((w+1)); done`,
		Did: made,
		Why: "the substitution's child is a second place the gate has to reach",
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
		Name:   "read/procsub",
		Only:   []string{"bash", "zsh", "ksh"},
		Script: `read L < <(read x < {{secret}}; echo $x); echo $L`,
		Did:    leaked,
		Why:    "the read half of a substitution, whose child is a runner of its own",
	}, {
		Name:   "read/procsub-tempfile",
		Only:   []string{"zsh"},
		Script: `read L < =(read x < {{secret}}; echo $x); echo $L`,
		Did:    leaked,
		Why:    "the substitution that materializes a file, so it writes before it reads",
	}, {
		Name: "read/coproc",
		Only: []string{"bash"},
		Script: `coproc { read x < {{secret}}; echo $x; }
read -r L <&${COPROC[0]}; echo $L`,
		Did: leaked,
		Why: "a coprocess is a child the parent talks to, and it carries the gate or it does not",
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
		Name:   "spelling/read-nfd-dir",
		Script: `read -r x < {{carvedaccentnfd}} && echo "$x"`,
		Did:    leaked,
		Why:    "the denied directory written the other way — one name, two byte strings",
	}, {
		Name:   "spelling/write-nfd-dir",
		Script: `echo overwritten > {{carvedaccentnfd}}`,
		Did:    accentOverwritten,
		Why:    "and the write half, which is the one that does damage",
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
func accentOverwritten(f Fixture, _ Outcome) bool {
	return changed(f.CarvedAccent)
}

func overwritten(f Fixture, _ Outcome) bool {
	return changed(f.Carved)
}

func changed(path string) bool {
	b, err := os.ReadFile(path)
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
		Name:   "module/zf_ln-sym",
		Only:   zsh,
		Script: `zmodload zsh/files; zf_ln -s {{secret}} {{target}}`,
		Did:    made,
		Why:    "#1819: a symbolic link is a write, and a link is the shape the walk answers",
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
		// These two were the ledger #1808 asks for, and they are what the
		// ledger is *for*: both sat inert with the note "will need a gate
		// when it is", and #2260 landed the module with the gate in the same
		// change, so they moved to contained rather than to ESCAPED.
		//
		// The module is one parameter and no commands at all, which makes it
		// the sharpest of these rows: a read is an expansion and a write is
		// an assignment, so a boundary watching commands sees neither.
		Name:   "module/mapfile-read",
		Only:   zsh,
		Script: `zmodload zsh/mapfile; echo ${mapfile[{{secret}}]}`,
		Did:    leaked,
		Why:    "#2260: a subscript reads a whole file, and names no command",
	}, {
		Name:   "module/mapfile-write",
		Only:   zsh,
		Script: `zmodload zsh/mapfile; mapfile[{{target}}]=x`,
		Did:    made,
		Why:    "#2260: an assignment to an element creates the file it names",
	}, {
		// The third route the module opens, and the one with no path in it
		// to hang a check on: `${(k)mapfile}` is a readdir of the working
		// directory spelled as a parameter flag. It is graded against the
		// directory the fixture denies rather than a file, so it answers to
		// AllowList where the two above answer to the read and write gates.
		Name:   "module/mapfile-roster",
		Only:   zsh,
		Script: `zmodload zsh/mapfile; cd {{outside}} && echo ${(k)mapfile}`,
		// The same predicate `list/glob` uses, and for the same reason: what
		// the route produces is a *name*, so what says it worked is the name
		// of the denied file coming back rather than its contents.
		Did: func(_ Fixture, o Outcome) bool { return o.Says("secret") },
		Why: "#2260: the roster enumerates a directory and names no path",
	}, {
		// bash keeps a history list and writes a history file in a shell
		// nobody is sitting at — measured, `bash -c 'history -w out'` creates
		// the file even with an empty list — which is what makes this row
		// reachable where zsh's `fc -W` beside it is not. It was inert with
		// the note "not a builtin yet" until #2271 landed the builtin and its
		// gate together.
		Name:   "builtin/history-write",
		Only:   []string{"bash"},
		Script: `history -w {{target}}`,
		Did:    made,
		Why:    "#2271: a history file is a write to a path the script names",
	}, {
		// The append half, which is a different system call on a different
		// flag and would be a hole of its own: a gate on `-w` alone leaves
		// `-a` opening the same path with O_APPEND.
		Name:   "builtin/history-append",
		Only:   []string{"bash"},
		Script: `history -s x; history -a {{target}}`,
		Did:    made,
		Why:    "#2271: `-a` opens the same path the write letter does",
	}, {
		// And the read, which is the letter that brings a denied file's
		// contents *into* the shell where `history` will print them.
		Name:   "builtin/history-read",
		Only:   []string{"bash"},
		Script: `history -r {{secret}}; history`,
		Did:    leaked,
		Why:    "#2271: `-r` reads a file into a list the script can print",
	}, {
		Name:   "builtin/mapfile",
		Only:   []string{"bash"},
		Script: `mapfile -t A < {{secret}}; echo $A`,
		Did:    leaked,
		Why:    "reads through a redirection, so it answers to the redirection's gate",
	}, {
		// The row that needed the instrument fixed as well as the shell.
		//
		// It ran `-c` and could not have gone green however `fc -W` was
		// written: zsh writes a history file **only when the shell is
		// interactive**, measured 2026-09-12 and not on account of an empty
		// list — a list loaded by `fc -R` and printed by `fc -l` is still not
		// written from a plain `-c`. So `-i` is what makes this a route, and
		// the list has to be seeded, because an empty one writes nothing in
		// zsh too.
		//
		// `-i` reads no startup file of anybody's: the grader gives every run
		// a HOME inside the fixture. See Route.Args.
		Name:   "builtin/fc-write",
		Only:   zsh,
		Args:   []string{"-i"},
		Script: `HISTFILE={{target}}; SAVEHIST=10; print -s ` + SecretMark + `; fc -W`,
		Did:    made,
		Why:    "#2283: a history file is a write to a path the script names",
	}, {
		// The append letter, which opens the same path on a different flag —
		// a gate on `-W` alone would leave it open, the same shape bash's
		// `history -a` has beside `-w`.
		Name:   "builtin/fc-append",
		Only:   zsh,
		Args:   []string{"-i"},
		Script: `SAVEHIST=10; print -s ` + SecretMark + `; fc -A {{target}}`,
		Did:    made,
		Why:    "#2283: `-A` opens the same path the write letter does",
	}, {
		// And the read, which brings a denied file's contents into a list the
		// script can then print with `fc -l`. No write anywhere in it.
		Name:   "builtin/fc-read",
		Only:   zsh,
		Args:   []string{"-i"},
		Script: `fc -R {{secret}}; fc -l`,
		Did:    leaked,
		Why:    "#2283: `-R` reads a file into a list the script can print",
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
