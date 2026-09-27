// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"runtime"

	"github.com/blairham/sh/interp"
)

// The nine names that say **what machine this is, which build, and which
// session** — and had no parameter here at all.
//
// Split out of #4866's ledger as #4903. Each one's value is a fact about
// something outside the shell, which is what separates them from the twelve
// constants #4912 supplied: writing a constant down was a measurement there,
// and here it would be a statement about one machine.
//
//	name            ${(t)} in the reference   where the answer lives
//	CPUTYPE         scalar                    the kernel's machine word
//	MACHTYPE        scalar                    the build triple
//	VENDOR          scalar                    the build triple
//	OSTYPE          scalar                    the build triple
//	ZSH_PATCHLEVEL  scalar                    the build
//	ZSH_SCRIPT      scalar                    the invocation
//	LOGNAME         scalar-export             the session
//	USERNAME        scalar-special            the process
//	TTY             scalar                    the session
//
// The four triple values are in buildtriple.go, which has the platform table
// and says which of its rows were measured. The five here are the ones whose
// source is this shell rather than this machine.
//
// # Nothing is asked of the system until something reads it
//
// `interp.LoginName` measures 0.83-1.10 ms on darwin — Directory Services,
// and no cheaper with cgo off — and `Apply` runs on every invocation, so a
// parameter that asked at startup would put that millisecond on every
// `zsh -c` and on every subshell (#1403, #2576). `$USERNAME` and `$LOGNAME`
// are therefore **produced**, and they share the one question `%n` already
// asks: one `sync.OnceValue` for all three, so a shell that draws a prompt
// and reads both names pays for one lookup.
//
// # The environment is an input, and it wins for exactly one of the nine
//
// Measured 2026-09-27, one shell per cell, `env -i PATH=/usr/bin:/bin` plus
// the one name set to `ZZ`, against `/opt/homebrew/bin/zsh`:
//
//	env OSTYPE=ZZ          darwin25.4.0   scalar-export
//	env CPUTYPE=ZZ         arm64          scalar-export
//	env MACHTYPE=ZZ        aarch64        scalar-export
//	env VENDOR=ZZ          apple          scalar-export
//	env ZSH_PATCHLEVEL=ZZ  zsh-5.9.2-…    scalar-export
//	env TTY=ZZ             (empty)        scalar-export
//	env USERNAME=ZZ        bhamilton      scalar-special
//	env ZSH_SCRIPT=ZZ      the script     scalar-export
//	env LOGNAME=ZZ         ZZ             scalar-export
//
// So the shell's own value wins for eight of them **and the `export` the
// environment brought stays on the name** — which is why the four triple
// values and the patch level are written with [interp.Runner.SetVar] rather
// than with SetSpecial: the value is replaced and the attribute is left
// alone, where SetSpecial would have taken the export word off and made
// every one of those rows read `scalar`.
//
// `LOGNAME` is the exception, and it is the row the whole group would have
// been written wrongly from: a rule read off the other eight would have made
// the shell's answer win there too.
//
// # `$LOGNAME` is not `$USERNAME` under another name, and that is measured
//
// In one shell under `env -i`, on this machine: `$LOGNAME` is `root` while
// `$USERNAME` is `bhamilton` and `id -un` is `bhamilton`. So the reference's
// fallback is the **session's** login name — what `logname` prints, which is
// `root` here too — and not the password entry for the uid.
//
// **Go's standard library has no `getlogin`**, and `os/user.Current` answers
// the uid's name, which is the other value. What is written here is therefore
// the uid's name, which is what the two agree on in an ordinary login session
// and differs in a process tree like this one's. It is said in place rather
// than left to be found, and the divergence is filed.
//
// The value the environment carries is what a real session reads either way —
// `LOGNAME` is exported by `login` — so the fallback is reached only under
// `env -i`, `sudo -i`, a container or a daemon-started shell, which is the
// same list #1446 records for `%u`.
//
// # `$ZSH_SCRIPT` is absent rather than empty off a script file
//
// Measured: `${+ZSH_SCRIPT}` is **0** under `-c` and on standard input, and
// 1 with a script file, where the value is the path **as the caller wrote
// it** — `zs.zsh` for a relative operand and `./zs.zsh` for one written that
// way. That is a presence question rather than a value, so it is
// [interp.Runner.SetDynamicPresence] and not an empty string; a name that
// existed holding nothing would make `${ZSH_SCRIPT-nope}` answer the wrong
// arm.
//
// The front end fills the path in **after** `Apply` has run, so a startup
// store could not have held it whatever the value was. A producer reads it at
// the moment of the read, which is also what keeps `$0` and this parameter
// from parting when a function moves `$0`.
//
// # `$TTY` is the terminal's path, and empty is a real answer
//
// Empty with no terminal, which is every case in this tree and every script
// in a pipeline. The path comes from [interp.Runner.TerminalName], which asks
// the descriptor what it is called through the package the gate already uses
// for that — the standard library has no `ttyname` either.
func registerTheIdentityValues(r *interp.Runner, loginName func() string) {
	t := tripleFor(runtime.GOOS, runtime.GOARCH, kernelRelease())
	for _, p := range [...]struct{ name, value string }{
		{"CPUTYPE", t.cpu},
		{"MACHTYPE", t.machine},
		{"VENDOR", t.vendor},
		{"OSTYPE", t.ostype},
		// The build, in the shape the reference writes and from the one place
		// this shell's version is stated: `$ZSH_VERSION`, `--version` and
		// this are the same claim, and a shell whose version depends on how
		// it was asked is the thing zshVersion's own comment guards against.
		// The reference's word is its packager's — `zsh-5.9.2-0-gddee3e7`
		// from a source build, `debian/5.9-4+b15` from a distribution's —
		// so there is no shape to copy, only a claim to make once.
		{"ZSH_PATCHLEVEL", "zsh-" + zshVersion},
	} {
		// SetVar and not SetSpecial: the value is this shell's whatever it
		// was handed, and an `export` the environment brought stays on the
		// name. See the table above, where every one of these reads
		// `scalar-export` in a shell that was handed the name.
		r.SetVar(p.name, p.value)
	}

	// The name this process runs as, asked once and only when something
	// reads it. `special`, which is the shell maintaining the parameter:
	// measured, `${(t)USERNAME}` is `scalar-special` with the environment
	// setting the name and without.
	//
	// **And no [interp.Runner.MarkShellOwnParameter] beside it**, which is
	// not an omission: a producer is already that statement — the mark is
	// for a parameter with an ordinary stored value, as its own comment
	// says. The line was here and a mutation that took it away killed
	// nothing, which is what says it was saying nothing. What the three
	// names below need is the *opposite* statement, and that one is not
	// derivable.
	r.SetDynamic("USERNAME", func(*interp.Runner) string { return loginName() })
	r.SetDynamicDeclaration("USERNAME", interp.ProducedDeclaration{})

	// The session's login name, exported, with the environment's value
	// winning — the one row of the nine where it does.
	//
	// Captured here rather than read in the producer because the producer
	// runs after a script may have assigned: Assigned is the later word and
	// this is the earlier one, so the two are asked in that order.
	inherited, handedIn := r.GetVar("LOGNAME")
	r.SetDynamic("LOGNAME", func(rr *interp.Runner) string {
		if written, ok := rr.Assigned("LOGNAME"); ok {
			return written
		}
		if handedIn {
			return inherited
		}
		return loginName()
	})
	// The export is the shell's own and not the environment's: measured, a
	// shell started with `env -i` still writes `export LOGNAME=…`.
	r.MarkExported("LOGNAME")
	r.SetDynamicDeclaration("LOGNAME", interp.ProducedDeclaration{})

	// The terminal this shell holds, by name, and empty where it holds none.
	r.SetDynamic("TTY", func(rr *interp.Runner) string {
		name, _ := rr.TerminalName()
		return name
	})
	r.SetDynamicDeclaration("TTY", interp.ProducedDeclaration{})

	// The script this shell was given, as the caller wrote it — and **absent**
	// where the program did not arrive as a named file.
	r.SetDynamic("ZSH_SCRIPT", func(rr *interp.Runner) string { return rr.ScriptFile() })
	r.SetDynamicPresence("ZSH_SCRIPT", func(rr *interp.Runner) bool { return rr.HasScriptFrame() })
	r.SetDynamicDeclaration("ZSH_SCRIPT", interp.ProducedDeclaration{})

	// And three of the four produced names above are **not** the shell's own,
	// which has to be said because this engine derives that word from a
	// shape: a producer means `special`, and these three are produced for
	// reasons the reference does not share — the terminal and the script
	// path are not known when a dialect registers, and the login name costs
	// a millisecond nothing should pay until it is read. Measured, all four
	// are ordinary stored scalars there and only `USERNAME` says `special`.
	// See interp.Runner.MarkParameterNotTheShellsOwn, and note that
	// `USERNAME` is deliberately absent from this list: it is produced for
	// exactly the same reason `LOGNAME` is and *is* the shell's own, which
	// is what says the production decides nothing.
	for _, name := range [...]string{"LOGNAME", "TTY", "ZSH_SCRIPT"} {
		r.MarkParameterNotTheShellsOwn(name)
	}
}
