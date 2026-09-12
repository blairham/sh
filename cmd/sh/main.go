// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Command sh drives the substrate.
//
// It is not the product. This repository is the core — a parser and an
// interpreter other programs embed — and the shell people run, with its own
// choice of dialect and its own interactive surface, is a separate thing
// built on top. What is here exists to exercise the library, to make its
// behavior inspectable, and to be a column in the conformance harness.
//
// It runs scripts, and the -dialect flag chooses which shell it is being:
// core refuses anything the panel disagrees about, and the others answer the
// way that shell does. That is what makes it a column in the conformance
// harness, which needs something it can hand a snippet to.
//
//	sh -h                        # what this is and how to invoke it
//	sh                           # a prompt, if stdin is a terminal
//	sh -c 'echo hi'              # run a command
//	sh script.sh a b             # run a script, with $1 and $2 set
//	sh < script.sh               # or on stdin
//	sh -dialect bash -c '…'      # be bash where the shells differ
//	sh -i                        # a prompt even where stdin is not a terminal
//	sh -e script.sh              # any set option, exactly as `set` reads them
//	sh -tokens 'echo hi'         # dump the token stream
//	sh -parse 'a && b'           # dump the syntax tree
//	sh -trace-events script.sh   # print every gated action to stderr
//	sh -deny /etc script.sh      # refuse every action at or under /etc
//	sh -deny signal script.sh    # refuse every signal the shell sends
//	sh -deny exec:/usr/bin/** …  # refuse one kind, under one subtree
//	sh -policy p.policy script.sh  # run it under a declarative policy
//	sh -audit log.jsonl script.sh  # record every action as JSON, one per line
//	sh -acp                        # serve the Agent Client Protocol on stdio
//	sh -plugin /opt/x/p script.sh  # a builtin whose process is not ours
//	sh -acp-connect npx pkg --acp  # drive an ACP agent, under the same policy
//	sh -blocks-list                # the recent blocks: what ran, and how it went
//	sh -blocks-show 1              # the most recent block, its record and output
//
// Everything a shell reads at invocation — `-c`, a script path and its
// positional parameters, `-s`, a lone `-`, set options like `-e` — is read
// by the shared front end in driver, exactly as the dialect binaries read
// it. Only the flags no shell has — -tokens, -parse, -dialect,
// -trace-events, -deny, -policy, -audit, -plugin, -blocks-list and
// -blocks-show — are this binary's own, and they
// come first on the line: the first word that is not one of them belongs to
// the shell, so a script's own arguments can never be mistaken for them.
//
// All four of the last ones reach the gate and event seam docs/design.md
// describes, and they exist because a seam nothing reaches is a seam nothing
// grades: the interpreter has asked a Gate about every exec, open, stat and
// directory read since its first commit, and until these flags no shipped
// binary ever set one, so a hole in the boundary would have looked exactly
// like a shell that works.
//
// -blocks-list and -blocks-show read the store a prompt records into — see
// docs/design/blocks.md. They are here rather than as a builtin for the reason
// the others are here: a builtin would have to live in interp, which has no
// history and no business acquiring one, and no shell in the panel has such a
// command for a dialect to claim. Both read a store instead of running a
// shell, so they end the invocation — through whatever gate the same line
// asked for, because a policy that hides the store hides it from the tool that
// reads it too.
//
// -trace-events and -deny are the debug half — a way to watch the gate refuse
// something. -policy and -audit are the shipped half: a declarative rule set
// read from a file, and the event stream written down in the schema its
// consumers share. docs/design/sandboxing.md is the specification for both.
//
// The two halves share a rule language rather than each having one. A -deny
// value is a policy rule minus its decision word, spelled with a colon because
// a flag value is one shell word, and a bare path is the shorthand it has
// always had. Two rule languages over one gate would be two answers to the same
// question, and the one nobody exercises is the one that is wrong.
//
// Neither half contains a *process*. The boundary is drawn around the
// interpreter: a policy refuses what the shell itself opens, stats and runs,
// and a command the shell was allowed to start makes its own accesses that
// nothing here sees — `allow exec /bin/cat` is `allow read /**` spelled less
// obviously. Containing a running child is the job of an OS sandbox, which
// sits above the substrate.
//
// A policy is never discovered: no environment variable, no dotfile. One that
// could be named by the environment could be replaced by anything able to set
// it, the sandboxed script included.
//
// -h, -help and --help are the exception to "flags no shell has", and the
// exception is deliberate. bash and zsh read `-h` as `set -h` and dash refuses
// it; none of them would be a useful answer from a binary that is a harness
// column and an inspection tool rather than a shell anyone is being bash for.
// Fidelity to `bash -h` belongs to cmd/bash, which reaches driver without
// passing through here at all. A person meeting this binary types -h, and a
// usage message is not shell behavior — so it is answered before a dialect is
// resolved, before an axis is consulted and before a policy is read.
//
// The default dialect stays `core`, which refuses at every axis the panel
// disagrees about, and that refusal is the point of the binary rather than a
// rough edge on it. Defaulting to `posix` would trade it for a worse failure:
// measured, that dialect cannot parse `a=(1 2 3)`, `[[ -n x ]]` or `${s:1:3}`,
// all of which every shell in the panel but dash accepts, so `sh -c` would
// stop at the front door on syntax rather than at the disputed semantics.
// Defaulting to `bash` would make the substrate's own driver answer every
// disputed axis the way one member of the panel does, which is the bash-first
// design this repository exists not to be, and would make the conformance
// harness's explicit `-binargs "-dialect bash"` indistinguishable from no
// choice at all. What the refusal owed a person was the name of the flag, and
// that is what it now carries — see axisRemedy.
//
// With nothing to run and a terminal on stdin it prompts: a line editor with
// history that survives the session, Tab completion of commands and files,
// PS1 and PS2, and a continuation prompt for a construct that has not
// finished, and job control: ^Z stops what is running and gives the prompt
// back, `jobs` lists what is stopped, and `fg` and `bg` put one back.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

const exitFailure = 2

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole of main, with the process's streams passed in rather than
// reached for.
//
// It is a function so a test can invoke the binary the way a person does:
// flags, dialect, seams, front end, status. The wiring in installSeams was
// unreachable from a test while this lived in main(), which is the failure
// this file has already had once — a shell that was never handed its gate
// looks precisely like a shell that works, and so does a main() that reads a
// policy and drops the error on the floor.
//
// stdin is here for the one route that reads a stream of its own rather than
// leaving it to the front end: `-acp` is a protocol server, and its input is
// the client's messages. Every other route leaves standard input to driver,
// which fills a nil one in with the process's — a shell's input is a
// descriptor its children inherit, and an io.Reader that is not an *os.File
// would put a copying pipe between every command and the terminal. So a
// caller with no protocol to serve may pass nil, and the tests on the other
// routes do: nothing but `-acp` reads this stream.
//
// It was reached for inside serveACP until #1335, and that is not a detail:
// a route that reads the process's own streams cannot be driven by a test at
// all, which is why `-acp -policy p` went unmeasured until somebody ran it by
// hand.
func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	own, rest, err := readOwnFlags(argv[1:])
	// Before the error, and before any dialect is resolved: a usage message is
	// not shell behavior, so it must not be reachable only through a shell that
	// works. `sh -h` used to be refused at an axis — `set -h` being an option
	// letter at all is one the panel disagrees about — which made the one
	// binary a person cannot casually run also the one that would not say how
	// to run it.
	if own.help {
		usage(stdout, argv[0])
		return 0
	}
	if err != nil {
		return fail(stderr, err)
	}
	sh, err := pickDialect(own.dialect)
	if err != nil {
		return fail(stderr, err)
	}
	// What to tell a person who runs into an axis nothing answered. It is set
	// here because this is the binary that has the flag: interp may not name a
	// shell and driver has no flags of its own, so both carry the sentence
	// rather than composing it. A dialect binary leaves it empty — telling its
	// user to choose a dialect would be telling them to fix our bug.
	sh.AxisRemedy = axisRemedy
	// The fallback for an argv with nothing in it; driver names the shell by
	// argv[0] the way every dialect binary is named.
	sh.Name = "sh"
	sh = withHighlighting(sh, own)
	if len(own.plugins) > 0 {
		// A plugin host relays the plugin's standard error onto this one from
		// a goroutine of its own, so from here on more than one goroutine
		// writes these streams and they have to be serialized. See
		// guardedStreams, and note that it is a no-op for the *os.File pair
		// the shipped binary runs with — this is about an embedder's writer.
		//
		// Before installSeams and before the streams reach the front end, so
		// there is one guarded pair rather than a guarded copy beside an
		// unguarded one.
		//
		// The condition is a cost rather than a behavior, and is deliberately
		// not covered: making it unconditional is a mutant that survives,
		// because with no plugin there is no second goroutine and the guard
		// excludes nothing that was not already excluded. It says what the
		// lock is for.
		stdout, stderr = guardedStreams(stdout, stderr)
	}
	sh, closer, err := installSeams(sh, own, stderr)
	if err != nil {
		// A policy that will not load is not a shell that runs unsandboxed.
		return fail(stderr, err)
	}
	sh, pluginsCloser, err := launchPlugins(sh, own.plugins, stderr)
	if err != nil {
		// Fatal, and see launchPlugins: a shell that quietly ran without the
		// plugin you asked for would resolve that name from PATH instead.
		if closer != nil {
			_ = closer.Close()
		}
		return fail(stderr, err)
	}
	// Both closers, everywhere the invocation can end. Written as one function
	// rather than repeated at each return, because the failure this file has
	// already had once is a cleanup that exists on some paths.
	done := func() {
		if pluginsCloser != nil {
			_ = pluginsCloser.Close()
		}
		if closer != nil {
			_ = closer.Close()
		}
	}
	if own.acpConnect {
		// The other direction: this shell drives an agent rather than being
		// one. The words after the flag are the command that starts it, so
		// they are not a shell invocation either — and the seams are already
		// installed, so a policy governs what the agent asks us to do exactly
		// as it governs what a script does.
		code := connectACP(sh, own.acpAllow, own.acpAuth, rest)
		done()
		return code
	}
	if own.acp {
		// Not a shell invocation at all: the process becomes an Agent Client
		// Protocol server on its own standard input and output, and the
		// shells it runs are sessions a client asks for. Reached before the
		// front end, because there is no argument vector for it to read —
		// everything about a session arrives as a message.
		//
		// The seams are already installed, which is the point of reaching it
		// here rather than earlier: a policy handed to `-acp` governs every
		// session the client opens, exactly as it governs a script.
		code := serveACP(sh, rest, stdin, stdout)
		done()
		return code
	}
	if own.blocksList > 0 || own.blocksShow != "" {
		// Reading a store is not running a shell, so this ends the invocation
		// before the front end is reached. The seams are already installed,
		// which is the point: a policy that hides the store hides it from the
		// tool that reads it too.
		show := func() error { return showBlock(sh, stdout, own.blocksShow) }
		if own.blocksShow == "" {
			show = func() error { return showBlocks(sh, stdout, own.blocksList) }
		}
		if err := show(); err != nil {
			done()
			return fail(stderr, err)
		}
		done()
		return 0
	}
	if own.tokens || own.parse {
		dump := dumpTree
		if own.tokens {
			dump = dumpTokens
		}
		if err := dump(stdout, strings.Join(rest, " "), sh.Dialect); err != nil {
			done()
			return fail(stderr, err)
		}
		done()
		return 0
	}
	// Everything else is a shell invocation, and the shared front end reads
	// it — this binary was the fifth copy of that logic once, and the copy
	// is what dropped a script's positional parameters, claimed `-f` for
	// "read this file" where every shell means noglob, and opened a file
	// named `-`.
	sh.Stdout, sh.Stderr = stdout, stderr
	code := driver.MainArgs(sh, append([]string{argv[0]}, rest...))
	// Closed here rather than deferred, because main ends with os.Exit and a
	// defer would never run. For the audit file nothing is lost either way —
	// every record is written straight through — but a file the process holds
	// open until the kernel takes it back is untidy in the way that later
	// reads as a leak. For a plugin it is not untidiness: a plugin the shell
	// did not shut down is a process that outlives it.
	done()
	return code
}

// ownFlags are the flags no shell has, so the shared front end must never
// see them: which dialect to be, the two dump modes, and the two that reach
// the gate and event seam.
type ownFlags struct {
	tokens, parse bool
	// help ends the invocation with a usage message, and is answered before
	// anything else: no dialect is resolved, no axis is consulted, no policy
	// is read. A shell that will not say how to invoke it is worse than one
	// that refuses to run.
	help        bool
	dialect     string
	traceEvents bool
	deny        []string
	policy      string
	audit       string
	acp         bool
	// blocksList is how many recent blocks to print, and blocksShow names one
	// to print in full. Both end the invocation: they read a store rather than
	// running a shell.
	blocksList int
	blocksShow string
	acpConnect bool
	acpAllow   bool
	// highlight colors the line as it is typed, at an interactive prompt.
	// Off by default, because that is what every real shell does.
	highlight bool
	// acpAuth names one of the authentication methods the agent advertises.
	// Empty attempts none: which credential a person signs in with is theirs
	// to choose, and picking one for them is the kind of silent default this
	// front end refuses everywhere else.
	acpAuth string
	// plugins are the plugin executables this invocation named, in the order
	// it named them. Repeatable, like -deny and unlike -policy: two plugins
	// are two sets of commands, which composes without ambiguity, where two
	// policies would be two rule sets and reading one of them silently is the
	// dangerous half of that.
	plugins []string
}

// readOwnFlags strips this binary's flags from the front of the line,
// leaving everything a shell would read.
//
// Only from the front: the scan stops at the first word that is not one of
// ours, so `sh script.sh -dialect` hands the script a parameter named
// `-dialect` rather than eating it, and `-c '…' -tokens` leaves the operand
// alone. The forms are the ones the flag package accepted — one dash or two,
// the value attached with `=` or as the next word.
func readOwnFlags(args []string) (own ownFlags, rest []string, err error) {
	own.dialect = "core"
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			break
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(a[1:], "-"), "=")
		switch name {
		case "h", "help":
			// The one letter this binary takes that a shell also has: bash and
			// zsh read `-h` as `set -h`, measured, and dash refuses it. Taken
			// anyway, and only here at the front of the line — `sh script.sh
			// -h` still hands the script a `-h`, and `bash -h` is cmd/bash's
			// to answer, which it still does through the front end untouched.
			// The letter is what a person reaches for, and hashall is what
			// they will never have meant by it on a binary whose whole flag
			// namespace is already its own.
			own.help = true
		case "tokens":
			own.tokens = true
		case "parse":
			own.parse = true
		case "dialect":
			if !hasVal {
				if i+1 >= len(args) {
					return own, nil, errors.New("-dialect requires an argument")
				}
				i++
				val = args[i]
			}
			own.dialect = val
		case "trace-events":
			own.traceEvents = true
		case "acp":
			own.acp = true
		case "acp-connect":
			own.acpConnect = true
		case "acp-allow":
			own.acpAllow = true
		case "highlight":
			own.highlight = true
		case "acp-auth":
			if !hasVal {
				if i+1 >= len(args) {
					return own, nil, errors.New("-acp-auth requires the id of an authentication method")
				}
				i++
				val = args[i]
			}
			own.acpAuth = val
		case "deny":
			if !hasVal {
				if i+1 >= len(args) {
					return own, nil, errors.New("-deny requires a path")
				}
				i++
				val = args[i]
			}
			// Repeatable rather than a separated list: every separator that
			// would do — a comma, a colon — is a character a path may
			// legally contain, so splitting one would refuse to deny some
			// directory that exists.
			own.deny = append(own.deny, val)
		case "blocks-list":
			// A count is optional, unlike every other value flag here, because
			// `-blocks-list` on its own is the thing people want and a
			// required argument would make the common case the long one. An
			// attached `=n` is how a different count is asked for; a bare next
			// word is not taken, since it would eat the script operand.
			own.blocksList = blocksListDefault
			if hasVal {
				n, cerr := strconv.Atoi(val)
				if cerr != nil || n <= 0 {
					return own, nil, fmt.Errorf("-blocks-list wants a positive count, not %q", val)
				}
				own.blocksList = n
			}
		case "blocks-show":
			if !hasVal {
				if i+1 >= len(args) {
					return own, nil, errors.New("-blocks-show requires a block id or a number")
				}
				i++
				val = args[i]
			}
			own.blocksShow = val
		case "plugin":
			if !hasVal {
				if i+1 >= len(args) {
					return own, nil, errors.New("-plugin requires the path of a plugin executable")
				}
				i++
				val = args[i]
			}
			// Repeatable, for the reason -deny is: every separator that would
			// do is a character a path may legally contain.
			own.plugins = append(own.plugins, val)
		case "policy", "audit":
			if !hasVal {
				if i+1 >= len(args) {
					return own, nil, fmt.Errorf("-%s requires a path", name)
				}
				i++
				val = args[i]
			}
			// Not repeatable, unlike -deny, and the difference is what a
			// second one would mean. Two deny paths are two rules; two
			// policies would be two rule sets, and while composing them is
			// well defined — deny wins, so the result is the intersection —
			// silently reading only one of a pair somebody wrote is the
			// dangerous half of that. One policy, named once.
			if name == "policy" {
				own.policy = val
			} else {
				own.audit = val
			}
		default:
			// Not ours — a set option, `-c`, an operand after `--` — so the
			// shell's own reading starts here.
			return own, args[i:], nil
		}
		i++
	}
	return own, args[i:], nil
}

// fail reports and returns the status a shell exits with when it was invoked
// wrongly, rather than exiting itself: a function that ends the process cannot
// be called from a test, and every caller here is on the path a test drives.
func fail(w io.Writer, err error) int {
	_, _ = fmt.Fprintln(w, "sh:", err)
	return exitFailure
}

// pickDialect resolves a name to a shell.
//
// The three vectors are chosen together because they answer different
// questions about the same shell: which constructs it accepts, what it means
// by them, and what it says when they fail. A driver.Shell is those three plus
// the two extension points, which is the whole of "which shell am I" — the
// same value each cmd/<shell> binary is built from, so this cannot answer
// differently from them.
//
// The prompt table travels with them, and that is a repair rather than
// decoration (#1421). A dialect's default prompts are values it holds — and,
// now that they are assigned into PS1 and PS2 where a person's run-commands
// file reads them, values a *script* can observe. Left out here,
// `sh -dialect bash` was a bash whose PS1 was unset where `./bash`'s was
// `\s-\v\$ `, and the conformance harness grades exactly that binary: the
// corpus would have scored the core driver down for a difference the dialect
// binary does not have. The editing, history and key-binding tables are
// deliberately still absent, because nothing a script can see depends on
// them.
//
// `core` is built the same way on both sides. The grammar refuses constructs
// not every shell has; the semantics refuses *behaviors* not every shell
// shares. A script that runs under it depends on nothing the panel disagrees
// about, which is a useful thing to be able to check and a poor way to run a
// shell — the same split docs/spec/core.md drew for strict POSIX.
func pickDialect(name string) (driver.Shell, error) {
	switch name {
	case "core":
		// Strict: what every shell agrees on is done, and anything they
		// disagree about is refused rather than silently given one shell's
		// answer. A portability check rather than a runtime.
		return driver.Shell{
			Dialect: syntax.Core(), Semantics: coreSemantics(),
			Diagnostics: interp.CoreDiagnostics(),
		}, nil
	case "posix":
		return driver.Shell{
			Dialect: syntax.POSIX(), Semantics: interp.PosixSemantics(),
			Diagnostics: interp.PosixDiagnostics(),
		}, nil
	case "bash":
		return driver.Shell{
			Dialect: bash.Dialect(), Semantics: bash.Semantics(),
			Diagnostics: bash.Diagnostics(), Register: bash.Apply,
			Prelude: bash.Prelude(), PromptStyle: bash.PromptStyle(),
		}, nil
	case "zsh":
		return driver.Shell{
			Dialect: zsh.Dialect(), Semantics: zsh.Semantics(),
			Diagnostics: zsh.Diagnostics(), Register: zsh.Apply,
			Prelude: zsh.Prelude(), PromptStyle: zsh.PromptStyle(),
		}, nil
	case "ksh":
		return driver.Shell{
			Dialect: ksh.Dialect(), Semantics: ksh.Semantics(),
			Diagnostics: ksh.Diagnostics(), Register: ksh.Apply,
			Prelude: ksh.Prelude(), PromptStyle: ksh.PromptStyle(),
		}, nil
	case "dash":
		return driver.Shell{
			Dialect: dash.Dialect(), Semantics: dash.Semantics(),
			Diagnostics: dash.Diagnostics(), Register: dash.Apply,
			Prelude: dash.Prelude(), PromptStyle: dash.PromptStyle(),
		}, nil
	}
	return driver.Shell{},
		fmt.Errorf("unknown dialect %q: want core, posix, bash, zsh, ksh or dash", name)
}

// coreSemantics is the core's own vector with the one answer this *binary*
// has to supply: what it says when asked for its version.
//
// The version option is a dialect's, and the core is not imitating a member of
// the panel — it is this program, which is asked `--version` by installers,
// editors and agents deciding whether a shell is usable, and a tool that
// cannot answer that is read as broken. The text can only be filled in here:
// interp holds no build's version, and the string below is written by the
// build — see version, in acp.go.
//
// The other dialects answer for themselves, including the one that refuses:
// `sh -dialect dash --version` is refused, because that is what dash does.
func coreSemantics() interp.Semantics {
	s := interp.CoreSemantics()
	s.VersionOption = interp.VersionOption{
		Spellings: "--version",
		Text:      fmt.Sprintf("sh %s (%s-%s)", version, runtime.GOARCH, runtime.GOOS),
	}
	return s
}

// printf writes one line of a dump.
//
// The error is dropped, deliberately and in one place rather than at every
// call: the only stream a failure to write the dump could be reported on is
// the stream the dump was going to.
func printf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// dumpTokens prints one token per line: position, kind, and for a word its
// spans with the quoting made visible, because the quoting is the part that
// decides what happens to a word later and the part hardest to see by eye.
func dumpTokens(w io.Writer, src string, d syntax.Dialect) error {
	l := syntax.NewLexer(src, d)
	for {
		t := l.Next()
		if t.Kind == syntax.TokEOF {
			break
		}
		printf(w, "%-8s %-12s %s\n", t.Pos, kindName(t.Kind), detail(t))
	}
	if err := l.Err(); err != nil {
		if l.Incomplete() {
			return fmt.Errorf("%w (input ends unfinished)", err)
		}
		return err
	}
	return nil
}

func kindName(k syntax.Kind) string {
	switch k {
	case syntax.TokWord:
		return "word"
	case syntax.TokIONumber:
		return "io-number"
	case syntax.TokNewline:
		return "newline"
	case syntax.TokArithCmd:
		return "arith-cmd"
	}
	return "operator"
}

func detail(t syntax.Token) string {
	if t.Kind == syntax.TokArithCmd {
		return "expr(" + t.Text + ")"
	}
	if t.Kind != syntax.TokWord {
		return t.Kind.String()
	}
	parts := make([]string, 0, len(t.Spans))
	for _, s := range t.Spans {
		parts = append(parts, spanLabel(s)+"("+s.Value+")")
	}
	return strings.Join(parts, " + ")
}

// spanLabel names a span by what it is and, where it matters, by the quoting
// it sits in. A substitution shown as "plain" would read as literal text,
// which is the opposite of what this tool is for.
func spanLabel(s syntax.Span) string {
	switch s.Kind {
	case syntax.CommandSubst:
		return quotePrefix(s) + "cmd-subst"
	case syntax.ArithSubst:
		return quotePrefix(s) + "arith"
	case syntax.ParamExp:
		return quotePrefix(s) + "param"
	}
	switch s.Quoting {
	case syntax.SingleQuoted:
		return "single"
	case syntax.DoubleQuoted:
		return "double"
	case syntax.DollarSingleQuoted:
		return "dollar-single"
	}
	return "plain"
}

// quotePrefix marks a substitution that sits inside double quotes, because
// that is what decides whether its result is field-split afterwards.
func quotePrefix(s syntax.Span) string {
	if s.Quoting == syntax.DoubleQuoted {
		return "quoted-"
	}
	return ""
}

// dumpTree prints the syntax tree, indented.
func dumpTree(w io.Writer, src string, d syntax.Dialect) error {
	p := syntax.NewParser(src, d)
	f := p.Parse()
	if err := p.Err(); err != nil {
		if p.Incomplete() {
			return fmt.Errorf("%w (input ends unfinished)", err)
		}
		return err
	}
	for _, st := range f.Stmts {
		printNode(w, st, 0)
	}
	return nil
}

func printNode(w io.Writer, n syntax.Node, depth int) {
	pad := strings.Repeat("  ", depth)
	switch x := n.(type) {
	case *syntax.Stmt:
		label := "stmt"
		if x.Background {
			label = "stmt &"
		}
		printf(w, "%s%-8s %s\n", pad, x.Pos(), label)
		printNode(w, x.Expr, depth+1)
	case *syntax.BinaryExpr:
		printf(w, "%s%-8s %s\n", pad, x.OpPos, x.Op)
		printNode(w, x.X, depth+1)
		printNode(w, x.Y, depth+1)
	case *syntax.Pipeline:
		label := "pipeline"
		if x.Negated {
			label = "pipeline !"
		}
		printf(w, "%s%-8s %s\n", pad, x.Pos(), label)
		for _, c := range x.Cmds {
			printNode(w, c, depth+1)
		}
	case *syntax.SimpleCmd:
		printf(w, "%s%-8s command\n", pad, x.Pos())
		for _, a := range x.Assigns {
			switch {
			case a.IsArray:
				var els []string
				for _, e := range a.Elems {
					els = append(els, e.Literal())
				}
				printf(w, "%s  %-8s assign %s=(%s)\n", pad, a.Pos(), a.Name, strings.Join(els, " "))
			case a.Index != nil:
				printf(w, "%s  %-8s assign %s[%s]=%s\n", pad, a.Pos(), a.Name,
					a.Index.Literal(), a.Value.Literal())
			default:
				printf(w, "%s  %-8s assign %s=%s\n", pad, a.Pos(), a.Name, a.Value.Literal())
			}
		}
		for _, arg := range x.Args {
			printf(w, "%s  %-8s word %s\n", pad, arg.Pos(), arg.Literal())
			printParams(w, arg, pad+"    ")
		}
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.Subshell:
		printf(w, "%s%-8s subshell\n", pad, x.Pos())
		printList(w, x.List, depth+1)
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.Group:
		printf(w, "%s%-8s group\n", pad, x.Pos())
		printList(w, x.List, depth+1)
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.TryClause:
		printf(w, "%s%-8s try\n", pad, x.Pos())
		printBranch(w, "try", x.Try, depth+1)
		printBranch(w, "always", x.Always, depth+1)
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.IfClause:
		printf(w, "%s%-8s if\n", pad, x.Pos())
		printBranch(w, "cond", x.Cond, depth+1)
		printBranch(w, "then", x.Then, depth+1)
		for _, e := range x.Elifs {
			printBranch(w, "elif-cond", e.Cond, depth+1)
			printBranch(w, "elif-then", e.Then, depth+1)
		}
		if x.HasElse {
			printBranch(w, "else", x.Else, depth+1)
		}
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.LoopClause:
		kw := "while"
		if x.Until {
			kw = "until"
		}
		printf(w, "%s%-8s %s\n", pad, x.Pos(), kw)
		printBranch(w, "cond", x.Cond, depth+1)
		printBranch(w, "do", x.Body, depth+1)
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.ForClause:
		items := "(no word list — iterates the positional parameters)"
		if x.HasItems {
			var ws []string
			for _, w := range x.Items {
				ws = append(ws, w.Literal())
			}
			items = "in " + strings.Join(ws, " ")
		}
		names := strings.Join(x.Names, " ")
		if x.RefusedName != "" {
			// A word carried here because it is not a name — the dialect
			// that checks it when the loop runs. Marked rather than printed
			// bare, so a dump does not read as though the loop had a
			// variable to bind.
			names = x.RefusedName + " (not a name)"
		}
		printf(w, "%s%-8s for %s %s\n", pad, x.Pos(), names, items)
		printBranch(w, "do", x.Body, depth+1)
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.CaseClause:
		printf(w, "%s%-8s case %s\n", pad, x.Pos(), x.Word.Literal())
		for _, it := range x.Items {
			var pats []string
			for _, w := range it.Patterns {
				pats = append(pats, w.Literal())
			}
			printf(w, "%s  %-8s pattern %s %s\n", pad, it.Pos(),
				strings.Join(pats, "|"), it.Term)
			printList(w, it.Body, depth+2)
		}
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.TestClause:
		printf(w, "%s%-8s test %s\n", pad, x.Pos(), condString(x.Expr))
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.ArithCmdClause:
		printf(w, "%s%-8s arithmetic %s\n", pad, x.Pos(), arithString(x.Parsed))
		printRedirs(w, x.Redirs, pad, depth)
	case *syntax.FuncDecl:
		kw := ""
		if x.Keyword {
			kw = " (function keyword)"
		}
		printf(w, "%s%-8s func %s%s\n", pad, x.Pos(), x.Name, kw)
		printNode(w, x.Body, depth+1)
	default:
		printf(w, "%s%-8s %T\n", pad, n.Pos(), n)
	}
}

func printList(w io.Writer, list []*syntax.Stmt, depth int) {
	for _, s := range list {
		printNode(w, s, depth)
	}
}

func printBranch(w io.Writer, label string, list []*syntax.Stmt, depth int) {
	printf(w, "%s%s:\n", strings.Repeat("  ", depth), label)
	printList(w, list, depth+1)
}

func printRedirs(w io.Writer, rs []*syntax.Redirect, pad string, depth int) {
	for _, r := range rs {
		n := ""
		if r.N != nil {
			n = r.N.Literal()
		}
		printf(w, "%s  %-8s redirect %s%s %s\n", pad, r.Pos(), n, r.Op, r.Word.Literal())
		if r.Heredoc != nil {
			kind := "expanded"
			if r.Heredoc.Spans[0].Quoting != syntax.Unquoted {
				kind = "literal"
			}
			for _, line := range strings.Split(strings.TrimRight(r.Heredoc.Literal(), "\n"), "\n") {
				printf(w, "%s    %-8s heredoc(%s) %s\n", pad, "", kind, line)
			}
		}
	}
	_ = depth
}

// printParams shows the parsed form of any ${ } inside a word. A word's
// Literal() flattens them, which is exactly what hides whether they were
// understood.
func printParams(out io.Writer, w *syntax.Word, pad string) {
	for _, s := range w.Spans {
		if s.Kind == syntax.ArithSubst && s.Arith != nil {
			printf(out, "%sarith %s\n", pad, arithString(s.Arith))
			continue
		}
		if s.Kind != syntax.ParamExp || s.Param == nil {
			continue
		}
		e := s.Param
		desc := "param " + e.Name
		if e.Length {
			desc = "param length-of " + e.Name
		}
		if e.Indirect {
			desc = "param indirect " + e.Name
		}
		if e.Index != nil {
			desc += "[" + e.Index.Literal() + "]"
		}
		if e.Op != syntax.ParamNone {
			colon := ""
			if e.Colon {
				colon = ": (empty counts as unset)"
			}
			desc += fmt.Sprintf("  op %q%s", e.Op.String(), colon)
		}
		printf(out, "%s%s\n", pad, desc)
		if e.Arg != nil {
			printf(out, "%s  arg %s\n", pad, wordShape(e.Arg))
			printParams(out, e.Arg, pad+"    ")
		}
		if e.Arg2 != nil {
			printf(out, "%s  arg2 %s\n", pad, wordShape(e.Arg2))
		}
	}
}

// wordShape renders a word so a substitution in it is not mistaken for
// literal text. Literal() flattens them, which is what hides the difference
// that matters: an operand is a word and is expanded.
func wordShape(w *syntax.Word) string {
	var parts []string
	for _, s := range w.Spans {
		switch s.Kind {
		case syntax.CommandSubst:
			parts = append(parts, "$("+s.Value+")")
		case syntax.ArithSubst:
			parts = append(parts, "$(("+s.Value+"))")
		case syntax.ParamExp:
			parts = append(parts, "${"+s.Value+"}")
		default:
			parts = append(parts, s.Value)
		}
	}
	return strings.Join(parts, "")
}

// arithString renders an expression fully parenthesised, so precedence is
// visible rather than implied. That is the point of parsing it at all: the
// source text was already in Expr.
func arithString(e syntax.ArithExpr) string {
	switch x := e.(type) {
	case nil:
		return "(unparsed)"
	case *syntax.ArithNum:
		return x.Text
	case *syntax.ArithVar:
		return x.Name
	case *syntax.ArithCharCode:
		if x.Subscripted {
			return x.Op + x.Name + "[…]"
		}
		return x.Op + x.Name + x.Char
	case *syntax.ArithOutput:
		return x.Text + " " + arithString(x.X)
	case *syntax.ArithUnary:
		if x.Postfix {
			return "(" + arithString(x.X) + x.Op + ")"
		}
		return "(" + x.Op + arithString(x.X) + ")"
	case *syntax.ArithBinary:
		return "(" + arithString(x.X) + " " + x.Op + " " + arithString(x.Y) + ")"
	case *syntax.ArithCond:
		return "(" + arithString(x.Cond) + " ? " + arithString(x.Then) + " : " + arithString(x.Else) + ")"
	case *syntax.ArithAssign:
		return "(" + x.Name + " " + x.Op + " " + arithString(x.Value) + ")"
	}
	return "?"
}

// condString renders a condition fully parenthesised, so the precedence is
// visible — and inside [[ ]] it is not the precedence the same operators have
// outside, which is exactly the thing worth being able to see.
func condString(c syntax.CondExpr) string {
	switch x := c.(type) {
	case nil:
		return "(empty)"
	case *syntax.CondUnary:
		return "(" + x.Op + " " + x.X.Literal() + ")"
	case *syntax.CondBinary:
		rhs := x.Y.Literal()
		if !x.Y.IsQuoted() && (x.Op == "==" || x.Op == "=" || x.Op == "!=") {
			rhs += " [pattern]"
		}
		if !x.Y.IsQuoted() && x.Op == "=~" {
			rhs += " [regex]"
		}
		return "(" + x.X.Literal() + " " + x.Op + " " + rhs + ")"
	case *syntax.CondLogic:
		return "(" + condString(x.X) + " " + x.Op + " " + condString(x.Y) + ")"
	case *syntax.CondNot:
		return "(! " + condString(x.X) + ")"
	case *syntax.CondGroup:
		return "[" + condString(x.X) + "]"
	}
	return "?"
}
