// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/boundary"
	"github.com/blairham/sh/internal/event"
	"github.com/blairham/sh/internal/plugin"
	"github.com/blairham/sh/interp"
)

// -plugin PATH: a builtin whose implementation is in another process, in
// whatever language its author chose. See docs/design/plugins.md for the
// design and internal/plugin for the protocol.
//
// This is cmd/sh's flag and not driver's, for the reason -policy is: driver is
// the shared front end for binaries that claim to *be* bash or zsh, and no
// real shell has a -plugin, so putting it there would make `./bash -plugin`
// accept a flag bash rejects.
//
// It exists at all because a seam nothing reaches is a seam nothing grades.
// The embedder API is the primitive — a program embedding a Runner names
// plugin executables in Go and composes their registrations — and this is the
// route a person can drive from a command line.
//
// # A plugin is never discovered
//
// The rule is the policy's rule with more force behind it, because it is a
// rule about the *source*: a discovered policy is arbitrary rules, and a
// discovered plugin is arbitrary code.
//
// No environment variable, because anything nameable by $SH_PLUGINS is
// replaceable by anything that can set the environment — including the
// sandboxed script itself, on its way to invoking a nested shell. No plugin
// directory, because a scan of ~/.sh/plugins is a variable with extra steps
// and a scan relative to the working directory would make `cd` into a
// downloaded repository a code-execution primitive. No dotfile, no $ENV, no
// prelude, and in particular **there is no `plugin` builtin**: a script cannot
// load a plugin. That last one is the important one. If a script could load a
// plugin then `eval` could load a plugin, and the argument the whole gate seam
// rests on — that a policy applied outside the interpreter is walked around by
// the first `eval` — would run backwards.
//
// There is no signature, no checksum and no registry either, and that is a
// decision rather than a gap. A checksum verified by the same process that is
// about to exec the file is a check whoever controls the file can also update,
// and shipping one would suggest an assurance this design does not have. The
// assurance is that the path was written on a command line by a person, and
// that the exec must also satisfy the policy.

// launchPlugins starts every plugin the invocation named and composes their
// registrations into the shell.
//
// Started here, before the front end reads its arguments, because names have
// to be in the command table before the first command word is resolved: a name
// that appeared partway through a run would make `type foo` answer differently
// depending on what had already run.
//
// A plugin that does not come up is a fatal invocation error. The alternative
// is a shell that carries on and resolves that name from PATH instead, running
// something other than what the invocation asked for — which is the failure
// mode the policy format's "every parse failure is fatal" rule already rejects
// in its own domain.
func launchPlugins(sh driver.Shell, paths []string, stderr io.Writer) (driver.Shell, io.Closer, error) {
	if len(paths) == 0 {
		return sh, nil, nil
	}
	// The run needs an identity before the first plugin is launched, because
	// the exec that launches one is an audited access and a record naming no
	// run is a record nothing can be joined to. driver fills this in when it
	// is empty; filling it in here means the plugin's exec and the script's
	// own actions carry the same string.
	if sh.Session == "" {
		sh.Session = event.NewID(time.Now())
	}
	bound := boundary.Boundary{Gate: sh.Gate, Events: sh.Events, Session: sh.Session}

	hosts := &plugins{}
	for _, path := range paths {
		h, err := plugin.Launch(context.Background(), plugin.Options{
			Path:   path,
			Bound:  bound,
			Stderr: stderr,
		})
		if err != nil {
			// Everything already up comes down: a shell that failed to start
			// the third plugin must not run with the first two, because the
			// invocation asked for all of them.
			_ = hosts.Close()
			return sh, nil, fmt.Errorf("plugin %w", err)
		}
		hosts.hosts = append(hosts.hosts, h)
	}

	// Composed with whatever the dialect installed rather than replacing it,
	// and *after* it, so a plugin may replace a builtin a dialect registered.
	// docs/design/plugins.md flags that as the maintainer's to reverse; it
	// follows from the conservation rule, since the plugin surface is
	// Register's surface and Register permits replacement.
	dialectRegister := sh.Register
	sh.Register = func(r *interp.Runner) {
		if dialectRegister != nil {
			dialectRegister(r)
		}
		for _, h := range hosts.hosts {
			h.Register(r)
		}
	}
	return sh, hosts, nil
}

// plugins is every plugin an invocation started, closed as one.
//
// A closer rather than a defer for the reason the audit file is one: main ends
// with os.Exit and a defer would never run. It matters more here than there —
// an audit file left open is untidy, while a plugin left running is a process
// that outlives the shell that started it.
type plugins struct {
	hosts []*plugin.Host
}

func (p *plugins) Close() error {
	var errs []error
	for _, h := range p.hosts {
		errs = append(errs, h.Close())
	}
	return errors.Join(errs...)
}
