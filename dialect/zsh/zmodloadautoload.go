// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/blairham/sh/interp"
)

// `zmodload -a` registers a builtin to be loaded from a module on first use.
//
// It is a promise and not a load: nothing about the module is looked at when
// the line runs, so a module that does not exist is not this line's problem.
// Measured 2026-10-03 on zsh 5.9.2 under `-f`, one shell per row:
//
//	zmodload -a zsh/nosuch mb; echo $?          0, silent
//	  … mb; echo $?                             failed to load module `zsh/nosuch': …   1
//	  … mb; mb; echo $?                         the second is command not found   127
//	zmodload -a zsh/zutil zfoo; zfoo            module `zsh/zutil' has no such feature:
//	                                            `b:zfoo': autoload cancelled
//	                                            autoloading module zsh/zutil failed to
//	                                            define builtin: zfoo
//	                                            — and the script ends, at 1; zsh/zutil
//	                                            is left unloaded and the entry stays
//	zmodload -a zsh/zutil zformat; zformat …    the module loads and zformat runs
//	zmodload -a zsh/nosuch echo                 failed to add builtin `echo'   1
//	zmodload -a zsh/nosuch zformat              0 — zformat is a stub until zsh/zutil
//	                                            loads; after `zmodload zsh/zutil` the
//	                                            same line is failed to add builtin
//	zmodload -a zsh/nosuch                      zsh/nosuch: `/' is illegal in a builtin
//	zmodload -a m a/b c                         the same for a/b, c registered, 1
//	zmodload -a zsh/nosuch mb; type mb          mb is a shell builtin
//	zmodload -a zsh/nosuch mb; mb() { … }; mb   the function runs
//	zmodload -a zsh/nosuch mb; builtin mb       the load is attempted
//
// So a name given no builtin name of its own is both — `zmodload -a foo`
// registers `foo` from module `foo` — and a registration over one that is
// still only a promise replaces it, the later module winning.
//
// The two listings and the removal, from the same session:
//
//	zmodload -a                                 mb (zsh/nosuch)   and   foo
//	zmodload -La                                zmodload -ab zsh/nosuch mb
//	                                            zmodload -ab foo
//	zmodload -ua mb                             0, and `type mb` is mb not found
//	zmodload -ua zz                             zz: no such builtin   1
//	zmodload -ua                                what do you want to unload?   1
//
// `-L` with operands is not a listing: `zmodload -La m1 b1` registers, at 0.
// `-b` is the default kind and says nothing more than `-a` does — `-ab` and
// `-ub` are `-a` and `-ua` — and on its own it is `-b, -c, -f, and -p must be
// combined with -a or -u` at 1.
//
// # What the listing does not hold
//
// zsh's own listing starts with two dozen rows nobody registered — `bindkey
// (zsh/zle)`, `compadd (zsh/complete)`, `limit (zsh/rlimits)` — which are that
// build's modules declaring their builtins autoloadable. This listing holds
// what a script registered and nothing else, for the reason `$modules` leaves
// the same population out (see zshModulesView): those rows would describe
// modules this shell does not have as though it did. It follows that `-ua` on
// one of them is `no such builtin` here, where zsh removes the promise.
//
// A name this shell keeps withdrawn until its module loads — `zf_mkdir`
// before `zsh/files` — is registered and listed, but the call still finds the
// name withdrawn: the module's own gate answers first.

// zmodloadAutoloadStore is the registrations, builtin name to module, under a
// name no script can reach and cloned into a subshell with every other store.
const zmodloadAutoloadStore = zshEngineStorePrefix + "zmodload.autoload"

// zmodloadAutoloads is the registrations now.
func zmodloadAutoloads(r *interp.Runner) map[string]string {
	table, _ := r.GetAssoc(zmodloadAutoloadStore)
	if table == nil {
		table = map[string]string{}
	}
	return table
}

// zmodloadAutoloadedModules is every module a registration names, sorted —
// the population `$modules` writes as `autoloaded`.
func zmodloadAutoloadedModules(r *interp.Runner) []string {
	seen := map[string]bool{}
	for _, m := range zmodloadAutoloads(r) {
		seen[m] = true
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// zmodloadAutoload is `-a` with operands: the module, then the builtins it is
// to define, the module's own name standing in when none are given.
func zmodloadAutoload(r *interp.Runner, args []string) int {
	module, names := args[0], args[1:]
	if len(names) == 0 {
		names = []string{module}
	}
	status := 0
	for _, name := range names {
		if strings.ContainsRune(name, '/') {
			r.Diagnosef("%s: `/' is illegal in a builtin\n", name)
			status = 1
			continue
		}
		if zmodloadBuiltinDefined(r, name) {
			r.Diagnosef("failed to add builtin `%s'\n", name)
			status = 1
			continue
		}
		zmodloadPromise(r, name, module)
	}
	return status
}

// zmodloadBuiltinDefined is whether a name is a builtin that is really there,
// rather than a promise a later registration may replace.
//
// A promise is one of two things: a registration already in the table, or a
// builtin a module defines while that module is not loaded with it switched
// on — which is what `zformat` is before `zsh/zutil` loads, measured above.
func zmodloadBuiltinDefined(r *interp.Runner, name string) bool {
	if _, promised := zmodloadAutoloads(r)[name]; promised {
		return false
	}
	if !r.KnownBuiltin(name) {
		return false
	}
	feature := "b:" + name
	for module, features := range zmodloadFeatures {
		if containsWord(features, feature) && !containsWord(zmodloadEnabled(r, module), feature) {
			return false
		}
	}
	return true
}

// zmodloadPromise records the registration and puts the name in the table as
// a builtin that loads its module when it is called.
//
// What the name ran before is kept by the closure, because a promise over a
// module's own builtin — `zmodload -a zsh/zutil zformat` — has to hand the
// call to that builtin once the module is loaded.
func zmodloadPromise(r *interp.Runner, name, module string) {
	before, _ := r.Builtin(name)
	if kept, wasPromise := zmodloadKept(r, name); wasPromise {
		// A registration over a registration: what the earlier promise kept
		// is still the builtin to hand the call to, and the name's current
		// entry is only that promise.
		before = kept
	}
	table := zmodloadAutoloads(r)
	table[name] = module
	r.SetAssoc(zmodloadAutoloadStore, table)
	zmodloadSetKept(r, name, before)
	if before == nil && containsWord(zmodloadFeatures[module], "b:"+name) {
		// The module's own builtin, and one its gate keeps withdrawn until
		// it loads: there is nothing to hand the call to, and the gate is
		// what answers for the name. Recorded and listed, not put in the
		// table — see the note on withdrawn names above.
		return
	}
	r.Register(name, func(r *interp.Runner, ctx context.Context, args []string) int {
		return zmodloadKeepPromise(r, ctx, name, args)
	})
}

// zmodloadKeepPromise is a registered name being called: load its module, and
// run the builtin the module defines.
func zmodloadKeepPromise(r *interp.Runner, ctx context.Context, name string, args []string) int {
	module := zmodloadAutoloads(r)[name]
	feature := "b:" + name
	if features, known := zmodloadFeatures[module]; known && !containsWord(features, feature) {
		// A module this shell has that does not define the name: zsh says so
		// twice, leaves the module unloaded and the promise standing, and
		// ends the script.
		r.DiagnoseAsTheShellf("module `%s' has no such feature: `%s': autoload cancelled\n", module, feature)
		r.DiagnoseAsTheShellf("autoloading module %s failed to define builtin: %s\n", module, name)
		r.StopTheScript(1)
		return 1
	}
	kept, _ := zmodloadKept(r, name)
	zmodloadForget(r, name)
	if code := zmodloadLoad(r, zmodloadOpts{}, module); code != 0 {
		// The load reported itself. The promise is spent, so the name is not
		// a builtin any more — the next call is command not found.
		r.Unregister(name)
		return code
	}
	if kept == nil {
		r.Unregister(name)
		r.DiagnoseAsTheShellf("autoloading module %s failed to define builtin: %s\n", module, name)
		return 1
	}
	r.Register(name, kept)
	return kept(r, ctx, args)
}

// zmodloadSettlePromises is a module loading: every promised name the module
// defines is defined now, whichever module the promise named, so the name
// goes back to the module's builtin and leaves the listing. Measured: after
// `zmodload -a zsh/nosuch zformat; zmodload zsh/zutil`, a second `zmodload
// -a zsh/nosuch zformat` is failed to add builtin `zformat' — the load put
// the real one in place of the promise.
func zmodloadSettlePromises(r *interp.Runner, module string) {
	for name := range zmodloadAutoloads(r) {
		if !containsWord(zmodloadFeatures[module], "b:"+name) {
			continue
		}
		kept, _ := zmodloadKept(r, name)
		zmodloadForget(r, name)
		if kept != nil {
			r.Register(name, kept)
		}
	}
}

// zmodloadUnautoload is `-ua`: take promises back.
func zmodloadUnautoload(r *interp.Runner, names []string) int {
	if len(names) == 0 {
		r.Diagnosef("what do you want to unload?\n")
		return 1
	}
	status := 0
	for _, name := range names {
		if _, promised := zmodloadAutoloads(r)[name]; !promised {
			r.Diagnosef("%s: no such builtin\n", name)
			status = 1
			continue
		}
		zmodloadForget(r, name)
		r.Unregister(name)
	}
	return status
}

// zmodloadForget removes a registration from the table.
func zmodloadForget(r *interp.Runner, name string) {
	table := zmodloadAutoloads(r)
	delete(table, name)
	r.SetAssoc(zmodloadAutoloadStore, table)
	zmodloadSetKept(r, name, nil)
}

// zmodloadAutoloadListing is `-a` with nothing after it, as `name (module)`
// or the command that would register it; a name its module shares is written
// once.
func zmodloadAutoloadListing(r *interp.Runner, commands bool) int {
	table := zmodloadAutoloads(r)
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		module := table[name]
		switch {
		case commands && module == name:
			zmodloadPrintf(r, "zmodload -ab %s\n", name)
		case commands:
			zmodloadPrintf(r, "zmodload -ab %s %s\n", module, name)
		case module == name:
			zmodloadPrintf(r, "%s\n", name)
		default:
			zmodloadPrintf(r, "%s (%s)\n", name, module)
		}
	}
	return 0
}

// zmodloadKeptStore is, for each registration, a token naming the builtin
// the name ran before it — kept beside the table so that a subshell's copy of
// the table carries its own copy of this too.
const zmodloadKeptStore = zshEngineStorePrefix + "zmodload.autoload.kept"

// zmodloadKeptBuiltins is what the tokens name. A store holds strings and a
// builtin is a function, so the store holds the token and this holds the
// function; an entry is never reused, so a token in any runner's store names
// exactly the builtin it was written for.
var zmodloadKeptBuiltins struct {
	sync.Mutex
	next  int
	funcs map[string]interp.Builtin
}

// zmodloadKept is the builtin a registered name ran before it was registered,
// nil when it ran nothing, and false when the name is not registered.
func zmodloadKept(r *interp.Runner, name string) (interp.Builtin, bool) {
	tokens, _ := r.GetAssoc(zmodloadKeptStore)
	token, ok := tokens[name]
	if !ok {
		return nil, false
	}
	zmodloadKeptBuiltins.Lock()
	defer zmodloadKeptBuiltins.Unlock()
	return zmodloadKeptBuiltins.funcs[token], true
}

// zmodloadSetKept records what a registered name ran before, or with nil and
// no registration left, forgets the name.
func zmodloadSetKept(r *interp.Runner, name string, fn interp.Builtin) {
	tokens, _ := r.GetAssoc(zmodloadKeptStore)
	if tokens == nil {
		tokens = map[string]string{}
	}
	if _, promised := zmodloadAutoloads(r)[name]; !promised {
		delete(tokens, name)
		r.SetAssoc(zmodloadKeptStore, tokens)
		return
	}
	zmodloadKeptBuiltins.Lock()
	zmodloadKeptBuiltins.next++
	token := strconv.Itoa(zmodloadKeptBuiltins.next)
	if fn != nil {
		if zmodloadKeptBuiltins.funcs == nil {
			zmodloadKeptBuiltins.funcs = map[string]interp.Builtin{}
		}
		zmodloadKeptBuiltins.funcs[token] = fn
	}
	zmodloadKeptBuiltins.Unlock()
	tokens[name] = token
	r.SetAssoc(zmodloadKeptStore, tokens)
}
