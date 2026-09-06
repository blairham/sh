# Plugins: extending the shell from a process, in any language

A plugin is a separate program the shell launches and talks to over a
protocol, so that a command can be written in a language other than Go
and still be a *builtin* — able to reach the shell's own state, the way
`cd` and `read` do and the way an external program never can.

Read `docs/design.md` first, and the paragraph in it that says the
boundary is drawn around the interpreter and not around the process
tree, because every security claim below is bounded by it.

This is the design borrowed from prior work of our own. What is borrowed
is the shape — separate process, handshake, declared surface, a builtin
whose implementation lives on the other side of a pipe. What is *not*
borrowed is the transport, and the section that decides it is the
longest one here, because the obvious answer and the right answer turn
out to differ.

## The concern this design has to survive

The extension model is deliberately three ways, and `AGENTS.md` and the
doc comment on `interp.Runner.Register` both state it: **choose the
vectors, register a builtin, source a prelude.** A plugin transport is a
fourth way, and the honest objection to it is not that it is unwanted
but that a fourth way can erode the other three. If registering a
builtin in-process becomes the awkward path once plugins exist, the
objection was right and this document is the mistake.

So the design is written around a rule it can be checked against:

> **A plugin adds no capability. It relocates one across a process
> boundary.**

Every method in the protocol below corresponds to a seam this repository
already publishes — `interp.Builtin` and `interp.Sink` — and to nothing
else. A plugin that could do something no in-process Go value can do
would be a fourth extension mechanism rather than a remoting of the
second and it would be outside this design.

Three consequences fall out, and they are the whole answer to "what
belongs in a plugin".

**The in-process route stays strictly cheaper.** A Go builtin is the
same signature with no process to spawn, no gate consultation, no
serialization, no version handshake, no way for the peer to die, and the
whole of `*interp.Runner` rather than the named subset in this document.
Anything a plugin can do, a Go builtin can do more of, faster, and with
fewer ways to fail. That is not an accident of the current protocol; it
is the property that keeps way two from becoming awkward, and it is the
first thing to check if this design is ever extended.

**A prelude still wins outright.** A plugin registers builtins, so a
plugin's command sits exactly where a Go builtin sits in name
resolution — after functions, before PATH. A shell function shadows it,
which means way three beats way four by construction rather than by
convention.

**The core is untouched.** `syntax` and `interp` gain no field, no
interface, no import and no knowledge that plugins exist. Neither does
`driver`: the host composes with the `driver.Shell.Register` hook that
dialects already use, so the only package that changes is `cmd/sh`. To a
script, a plugin's command is a builtin — `type` says `builtin`, `enable
-n` switches it off — and that is deliberate in both directions. The
script cannot enumerate which of its commands are plugins, and `interp`
does not acquire a concept it has no use for.

## What a plugin is for, and what it is not for

The ordering advice on `Register` is unchanged and gains a fourth line:

1. **A shell function in a prelude.** Portable, testable with any shell,
   and it cannot break the substrate. Most of a dialect belongs here.
2. **A Go builtin**, when shell genuinely cannot say the thing — it has
   to reach the runner's own state.
3. **A plugin**, when the thing that cannot be said in shell also cannot
   be written in Go.

That last clause is narrow on purpose, and there are exactly two honest
readings of it.

**It already exists in another language.** The cost being avoided is a
rewrite, and it is a real cost. This is the case the initiative is for.

**It must not be linked.** A dependency too large to take, a license
that cannot be accepted, or a library unstable enough that its failures
would become the shell's. This one is worth naming because it is a
benefit the in-process route cannot offer at all: **a plugin cannot
panic the shell.** A Go builtin shares the address space and a `Register`
that misbehaves takes the process with it, panic guard or no.

What does **not** belong in a plugin:

- **Anything expressible as a shell function.** A prelude has none of
  the failure modes below and costs nothing.
- **Anything whose job is moving bytes.** A filter reading standard
  input and writing standard output is an ordinary external command, and
  an external command is already free — it inherits real descriptors and
  the kernel moves the data. The reason to be a builtin at all is that
  you need the *shell's* state; if you do not, do not be one. This is
  also the argument that lets the transport below be cheap, so it is
  load-bearing rather than advice.
- **Anything on a hot path.** Glob descent, word expansion, PATH search,
  a completion per keystroke. A round trip is microseconds and these
  happen thousands of times per line.
- **Anything that changes what the parser accepts.** A dialect is a
  table of values, re-read as execution proceeds — `set -o posix`
  halfway through a script governs the parse of the rest of it. A value
  supplied by a foreign process at an unpredictable moment is not a
  table; it is a negotiation, and the parser has nowhere to wait.

## The surface: two roles, and what is excluded

A plugin declares its roles at the handshake. There are two.

### The command role

The plugin declares a set of names. The host registers a stub for each
through `Runner.Register`, and the stub turns a call into a request:

| `interp.Builtin` | over the wire |
| --- | --- |
| `args []string` | `args` on the invoke request |
| `r.Out()`, `r.Err()` | output chunks, streamed back |
| `r.In()` | `shell/read`, which the plugin asks for — see below |
| return status `int` | the invoke response |

The input row was written the other way round — "input chunks, streamed in" —
and **it was wrong**, discovered while building it. Pushing input to a plugin
that never reads it makes the host's write block forever on a full pipe, which
is #690 in a new costume and is the exact shape the lifetime section below
forbids; there is no bound on it that is not a lie about how much input there
was. A read the plugin asks for cannot outrun the plugin. So input is
`shell/read` on the host-method list, and a plugin that never reads costs
nothing.

Plus the small part of the runner a plugin genuinely needs, and no more:

| host method | why it is here |
| --- | --- |
| `shell/getVar` | reading a shell variable is not otherwise possible from another process — the environment is the Runner's, not the process's |
| `shell/setVar` | this is the *reason* `Register` exists: `read` has to put a value into a variable of the calling shell |
| `shell/dir` | the shell's working directory is `r.Dir` and is not the process's, so a plugin resolving a relative path has no other way to ask |
| `shell/read` | the stream the command's standard input points at, which under a pipeline or a redirection is not the process's |
| `shell/diagnose` | so a plugin's complaint is located and named the dialect's way, rather than an unattributed line on stderr |

Every one of them carries the **call** it belongs to, and that is not
ceremony. A subshell has its own variables and its own directory, and the two
halves of a pipeline are two Runners; a host method that named no call would
read whichever of them answered most recently.

Deliberately **not** exposed, each for a stated reason:

- **`Register` / `Unregister` at call time.** A plugin rewriting the
  command table while a script runs is a shell whose `type` answer
  depends on what has already run. Names are declared at the handshake
  and fixed for the plugin's life.
- **`Expand`.** Expansion includes command substitution, so this is
  `eval` by proxy: it re-enters the interpreter with input a foreign
  process chose, while a builtin call from that process is in flight.
  Every re-entrancy hazard in the interpreter, reachable from outside
  it, to save a plugin author a `getVar`.
- **`LookPath`, `WriterForFd`, `NamedOption`, `FunctionText`.** Each
  exists for a specific in-process builtin. None is needed to write a
  command, and a surface is easier to widen later than to narrow.

`shell/setVar` needs one thing said plainly, because it looks like a
gap. It is **not** gated, and it should not be: setting a variable does
not leave the interpreter's own memory, and the action vocabulary is
deliberately the six kinds that do. Adding a seventh would be an `interp`
change first — the rule `docs/design/sandboxing.md` states about network
rules, applied here.

### The observer role

The plugin receives the event stream: `interp.Sink`, remoted. This is
nearly free, because the serialization already exists — `internal/event`
is the JSON Lines schema at version 1, with its five stability rules,
and it was designed as a shared contract rather than a sandbox feature.
The observer role sends exactly those records, as notifications.

One property of that schema turns out to matter more here than where it
was written. A `Sink` is called **synchronously on the emitting
goroutine**, so a remote sink that blocked would make an observer plugin
able to stop the shell. It therefore writes into a bounded buffer and
**drops** when the buffer is full rather than blocking. That is honest
only because `seq` exists: the schema's own words are that `seq` is for
"replay and gap detection — a consumer that has records 1..40 and then
42 knows it lost one". A dropped record is detectable by the consumer
that lost it, which is the difference between lossy and lying.

### Gate consultations are excluded, and this is the sharpest decision here

A plugin may **not** answer gate consultations. Three reasons, in
increasing order of how load-bearing they are.

**The rate is wrong by orders of magnitude.** A `PATH` search stats a
candidate in every directory `PATH` names; a glob stats every entry it
descends past. `docs/design/sandboxing.md` calls this "one of the
hottest things the interpreter does", and it made that argument to
justify a counter rather than sixteen bytes of randomness per action. A
process round trip per stat is not a slower shell, it is a different
one.

**There is no third answer.** #493 settled that a deadline on a
permission decision "silently allows or refuses" and so "reports
something other than what happened". A remote gate has that problem with
no way out, because unlike an ACP permission request there is nothing to
cancel — the interpreter is mid-stat with a decision outstanding. Fail
closed and a plugin that hangs bricks the shell; fail open and
`kill -9` on the plugin is a privilege escalation.

**The seam is already open, so nothing is lost.** An embedder that wants
a remote decision writes a `Gate` in Go that calls out however it likes.
`docs/design/sandboxing.md` says so in as many words: a `Gate`
implementation "may consult the kernel, prompt a person, or call out to
a supervisor, because the interface is `Allow(ctx, Action) Decision` and
nothing constrains how the answer is reached." What is being declined
here is *standardizing* that call, not permitting it. The person who
writes it owns its timeout, and owns the consequences of it.

### Completions are excluded, for a different reason

The in-process seam now exists — `repl.Completer`, `repl.Completion` and
`Shell.Completers`, landed on its own merits so that it would be shaped
by its in-process callers rather than by a wire protocol. That was the
prerequisite this section used to be waiting on, and it removes the
first half of the objection. **It does not remove the second.**

Completion happens per keystroke, which is the hot-path exclusion above,
and the seam is synchronous on the editor's own goroutine: a completer
that blocks blocks the person typing. `repl/completer.go` writes down why
there is no deadline and no goroutine behind it — a deadline reports
something other than what happened (#493), and abandoning a slow
completer leaks one goroutine per Tab (#690) — and that reasoning is
exactly what a remote completer would have to answer for.

So the third role is still not taken, and what it needs is now
statable rather than vague. A remote completer must carry a cancel
notification, must be droppable when it stops answering, and must not be
consulted on the keystroke path without one of those two. None of that
changes `repl.Completer`, which is the point of having landed it first:
the role is additive, per the version rules below.

## Security

### Launching a plugin is an exec, and it passes the exec gate

The host asks the gate for `ActionExec` with the plugin's resolved path
and its argv, before spawning it, through `internal/boundary` — which is
exactly what that package is for: a front end asking the interpreter's
gate about the front end's own accesses. It is a further caller of an
existing mechanism, not a new one, and it carries the run's `Session`
and an action id like every other front-end access.

Under a default-deny policy, **a plugin does not start unless the policy
names it.** `sh -policy p -plugin /opt/x/plugin script.sh` needs a rule
permitting the exec of `/opt/x/plugin`, in the same way the policy must
already permit reading `script.sh`. That is correct rather than
inconvenient: the person who wrote the policy is the person who named
the plugin.

Refusal is fatal to the invocation and not silent. See the lifetime
section — a shell that quietly runs without the plugin you asked for
will resolve that name from PATH instead, which is worse than not
starting.

The transport itself is not separately gated. It is a pipe pair to a
child, on no path the script chose; the exec is the access, exactly as
the open of a process substitution's pipe is the access and the mkfifo
that created it is not.

### A plugin is outside the boundary, exactly as much as any allowed exec

**A plugin's own file and process accesses do not pass the policy, and
they cannot.** The boundary is drawn around the interpreter. Once a
process exists it makes its own system calls and nothing in this
repository has a say over them.

This is not a weakness peculiar to plugins. It is the same sentence
`docs/design/sandboxing.md` writes about every allowed command:
`allow exec /bin/cat` is `allow read /**` spelled less obviously. So:

> **A plugin is trusted exactly as much as an allowed exec, and no more
> and no less.**

The reason to state it rather than imply it is that a plugin *looks*
like part of the shell — it registers builtins, its commands appear in
`type`, its diagnostics are worded the dialect's way — and none of that
makes it inside the boundary. Containing a plugin, like containing any
child, is the job of an OS sandbox above this layer.

Two residual facts follow, and both are recorded here rather than left
to be discovered:

**A plugin can read every shell variable.** `shell/getVar` is
ungated by the argument above, so a plugin sees variables naming paths a
policy hides files under. This is the "allowing an interpreter allows
everything" footgun in a second dress, and there is no parser check that
can detect it.

**Nothing a plugin does off the wire is in the audit trail.** What it
asks *us* for is recorded — the exec that launched it, and any future
host method that performs an access. What it does with its own
descriptors is invisible, and a trail that implied otherwise would report
more than it enforces.

### The callback surface is small on purpose, and the asymmetry with ACP is deliberate

`docs/design/acp.md` argues the opposite way for its client role: it
*implements* `fs/read_text_file` and `terminal/create` precisely so that
an agent's accesses come through our gate instead of happening
invisibly. The same reasoning does not transfer, and the difference is
worth naming because the two documents otherwise read as contradicting
each other.

An ACP client is constraining a third-party agent it launched under a
policy, and offering the agent a route through our boundary is a net
gain in visibility whenever the agent chooses to take it. A plugin is
already free to open whatever it wants, and offering it a route through
us would not remove that freedom — it would only add inward attack
surface, in the process that holds the script's variables and streams.
So the host methods are the four above, which touch the runner's memory
and not the world, and there is no `plugin/openFile` and no
`plugin/runCommand`.

## Discovery and trust

**A plugin is never discovered.** The rule is the policy's rule with
more force behind it, and it is a rule about the *source*, so it cannot
be relaxed by a plugin, a policy, or a script.

Two routes, and the split is the one `docs/design/sandboxing.md`
established:

**The embedder API is the primitive.** A program embedding a Runner
names plugin executables in Go, builds a host, and composes its
registrations. It may equally write the builtin in Go and never launch
anything.

**The invocation flag is the reachability route.** `cmd/sh` grows
`-plugin PATH`, repeatable, beside `-policy`, `-audit`, `-deny` and
`-trace-events`. It is `cmd/sh`'s own flag and not `driver`'s, for the
reason `-policy` is: `driver` is the shared front end for binaries that
claim to *be* bash or zsh, and no real shell has a `-plugin`, so putting
it there would make `./bash -plugin` accept a flag bash rejects. And it
exists at all because a seam nothing reaches is a seam nothing grades.

**The path must be absolute, and there is no PATH search.** A searched
name is a plugin whose identity depends on a variable, and `PATH` is a
variable a script sets. This is the same requirement the policy format
puts on a pattern, for the same reason.

Never, and each is a specific failure rather than a general caution:

- **No environment variable.** Anything nameable by `$SH_PLUGINS` is
  replaceable by anything that can set the environment — including the
  sandboxed script itself, on its way to invoking a nested shell. A
  policy named that way is arbitrary rules; a *plugin* named that way is
  arbitrary code, chosen by whoever set the variable.
- **No plugin directory.** A scan of `~/.sh/plugins` is a variable with
  extra steps, and a scan relative to the working directory would make
  `cd` into a downloaded repository a code-execution primitive.
- **No dotfile, no `$ENV`, no prelude.** And in particular **there is no
  `plugin` builtin**: a script cannot load a plugin. This is the most
  important sentence in the section. If a script could load a plugin
  then `eval` could load a plugin, and the argument the whole gate seam
  rests on — that a policy applied outside the interpreter is walked
  around by the first `eval` — would run backwards: a sandboxed script
  could bring in a process that is outside the boundary by construction.

**There is no signature, no checksum, and no registry, and that is a
decision.** A checksum verified by the same process that is about to
exec the file is a check whoever controls the file can also update, and
shipping one would suggest an assurance this design does not have. The
assurance is that the path was written on a command line by a person,
and that the exec must also satisfy the policy. A plugin is trusted
exactly as much as the person who wrote the invocation.

## Transport, and the dependency decision

This is the decision the issue title presumes and it deserves the
argument rather than the assumption. The goal is that **a plugin can be
written in any language**. What that actually requires is three things:
a separate process, a wire format any language can produce and consume,
and a transport any language can drive.

gRPC supplies all three, with generated stubs for a dozen languages,
bidirectional streaming, deadline propagation and a mature handshake
convention. It is the obvious answer. It is not the one this design
takes.

### What it would cost here, measured

**It ends stdlib-only.** Measured 2026-09-05: `go list -deps ./cmd/sh`
lists **zero** packages outside the standard library and this module.
Every entry in `go.mod`'s require block is `// indirect`, pulled in by
the `tool` block for golangci-lint and gofumpt, and none of it is in the
build graph of anything shipped. `google.golang.org/grpc` and
`google.golang.org/protobuf` would be the first runtime dependencies
this repository has ever had, and they bring `golang.org/x/net`,
`golang.org/x/sys` and `golang.org/x/text` with them.

**It adds a generator to the build.** `protoc` or `buf`, plus
`protoc-gen-go` and `protoc-gen-go-grpc`, pinned, reproducible, and run
in CI. `AGENTS.md` also requires the linter that understands a
technology to arrive **in the same change as the dependency** —
`protogetter` for protobuf — so that is part of the cost rather than an
afterthought.

**It raises the bar for exactly the authors this is for.** Writing a
gRPC server is comfortable in Go, Rust, Python, Java or Node. It is
unpleasant in a small C program, and it is not happening in awk, Tcl, or
a shell script. For a *shell's* extension system, the plausible first
plugin is a short script, and HTTP/2 framing plus protobuf is a wall in
front of the case with the most demand.

**And the claim it rests on is false in this repository.** "You need
gRPC to reach other languages" is falsified thirty lines away:
`internal/jsonrpc` is JSON-RPC 2.0 over a newline-delimited
stdio stream, written with no third-party dependency, and
`docs/design/acp.md` records it driving three separate agents — none of
them written in Go, all three launched with `npx`. A stdio protocol
reaching other-language processes is not a hypothesis here; it is
shipped and measured.

### What gRPC buys, taken one at a time

| gRPC gives | do we need it | what serves instead |
| --- | --- | --- |
| bidirectional streaming | yes — a call streams input in and output back | JSON-RPC is symmetric; `internal/acp`'s `Conn` "is a peer, not a side" and can call out while a handler runs |
| concurrent multiplexing | yes — a background job and each half of a pipeline call from their own goroutines | request ids; `Conn` already tracks concurrent in-flight calls |
| deadlines | **no** — see the lifetime section; cancellation is the way out | a cancel notification, which ACP already has as `session/cancel` |
| schema and codegen | genuinely useful | a JSON schema document, which is how ACP's own message shapes were read here — "from the machine-readable schema rather than from prose" |
| compact framing | not at this call rate | see below |

That last row is a consequence of an earlier decision rather than a
coincidence. Excluding the gate role removed the only per-syscall
traffic from the protocol; what remains happens **once per command a
script runs**, not once per stat a glob does. JSON is not the bottleneck
at that rate, and if it ever became one, the reason would be a plugin
doing something the "does not belong in a plugin" list already rules
out.

### Decision

> **The stdlib-only constraint holds. The transport is JSON-RPC 2.0
> over the plugin's standard input and standard output: UTF-8, one
> message per line, no embedded newlines — the same framing
> `internal/acp` already implements. gRPC and protobuf are not
> adopted.**

The design goal survives intact and is arguably better served: **a
plugin is a program that reads lines of JSON from standard input and
writes lines of JSON to standard output.** That is implementable in an
afternoon in any language on the machine, including several where a gRPC
server is not practical. It is a stated budget rather than a hope — if a
future addition to this protocol cannot be implemented in a page of
Python, the addition is wrong.

What is given up, honestly: no generated client stubs, so an author
writes a little framing by hand or takes a reference implementation we
ship. And **binary data costs**, which is the one place gRPC would have
been plainly better. JSON strings are UTF-8 and a shell's streams are
bytes, so stream chunks are base64 — a third more bytes and a copy. That
is acceptable precisely because of the rule stated earlier: a plugin
whose job is moving bulk bytes should be an ordinary external command,
which pays none of this. It is flagged below.

## Protocol

### The handshake

The **host speaks first**, with `initialize`. That ordering is
deliberate: a plugin that dies before saying anything is then a launch
failure with a diagnostic, rather than a host blocked on a first message
that will never come.

The request carries `protocolVersion`, an integer, `1`. The plugin
answers with the version it speaks, its name, and its declared surface:
the roles it takes and, for the command role, **every name it claims**.

### Negotiate capabilities; do not negotiate the version

The two rules are different and conflating them is how a protocol gets a
silent misinterpretation.

**A version mismatch is a refusal.** If the plugin answers with a
version the host does not know, the plugin is refused and said so. This
is the `version 1` rule of the policy format read at the protocol layer:
a policy half-understood "allows what it was written to refuse", and a
protocol half-understood produces a builtin that does something other
than what the script asked. A log tolerating an unknown record is a
different risk from code carrying the shell's name.

**A method or capability mismatch is a fact.** `docs/design/acp.md`
established both halves of this from measurement, and both apply
unchanged:

- **`-32601` is a fact, not a failure.** A plugin answering
  method-not-found is a plugin that does not do that thing; the host
  records it and stops asking. Likewise the host answers `-32601` to a
  host method it does not have, and a plugin must carry on.
- **An advertised capability is not a working method.** Degrade on the
  *answer*, not on the advertisement. A plugin declaring the command
  role and then answering `-32601` to an invoke has its registrations
  withdrawn — and the withdrawal is the same one a crash produces, so
  there is one recovery path rather than two.

**Unknown fields are ignored**, which is rule three of the event
schema's stability rules. Those rules are adopted wholesale rather than
restated, so there is one contract in this repository and not two.

### Stream purity, both ways

A plugin **must not** write anything to its own standard output that is
not a protocol message. This is ACP's rule, and it binds a plugin the
way it binds us when we are the agent.

Its standard error is free, and the host **relays it** to the shell's
`Stderr`, prefixed `sh: plugin <name>: `. Discarding it was considered
and rejected: a plugin dying with a message and the message being eaten
is the worst failure mode a plugin author meets, and the prefix keeps it
attributable in the middle of a script's own output. Flagged below.

There is no magic cookie in the handshake. A cookie in the child's
environment is not a secret from the child, so it is not a security
feature; it only stops a plugin binary hanging when a person runs it by
hand, and the plugin-author convention of printing a usage line when
standard input is a terminal does that better and needs no protocol.

## Failure and lifetime

#690 is the standing example of the class of bug this section exists
to prevent: code expected an open to fail where a FIFO actually
**blocks forever**, so a goroutine and an OS thread leaked per
occurrence, in a package whose whole purpose is being embedded in a
long-lived program. A plugin host is a richer version of the same
hazard — a peer process, pipes in both directions, and a partner that
can stop cooperating at any moment.

### Plugins start at startup, and a failure to start is fatal

Names have to be in the command table before the first command word is
resolved, or `type foo` would answer differently depending on what had
already run. So every `-plugin` is launched and handshaken before the
program does, which is also where the single exec gate consultation
belongs — once per plugin, not once per call.

**A plugin named on the command line that does not come up is a fatal
invocation error**, status 2, the front end's usage status. The
alternative is a shell that carries on and resolves that name from PATH
instead, running something other than what the invocation asked for,
which is the failure mode the policy format's "every parse failure is
fatal" rule already rejects in its own domain.

The cost is a process spawn per plugin at startup, and it is stated
rather than mitigated: a plugin you configured is a plugin you want.

### There is no per-call deadline, and cancellation is the way out

A plugin builtin may legitimately run for hours, so a default deadline
would break correct plugins to catch broken ones. Cancellation is what
the shell already has: `^C`, or the context the Runner was given being
cancelled, sends a cancel notification; if the plugin does not respond
within a bound the host kills it and the call fails.

The plugin runs in **its own process group**, which the interpreter
already knows how to arrange. Two things follow: `^C` at the prompt goes
to the foreground group and does not reach the plugin, so the host
decides what a cancellation means rather than racing the terminal; and
killing the plugin kills what the plugin started.

Bounded deadlines exist in exactly the two places where there is nothing
to cancel: the **handshake**, and **shutdown**.

### #493's rule about deadlines, and why it does not forbid this one

#493 concluded that a deadline which "silently allows or refuses
reports something other than what happened", and there is **no** timeout
on an ACP permission request for that reason. That rule is about a
*decision*, where a timeout must pick allow or deny and both are lies.

A timeout on a builtin call is not a decision and has a third answer
available, which is the true one: **the command failed.** It is reported
with a diagnostic and a status, and nothing is silent. That is why the
same document can forbid one and this one permit the other, and stating
the distinction is what stops the rule being copied into a place it
does not fit.

### What happens to an in-flight call when the plugin dies

1. **The call fails visibly.** The builtin returns **126** — the status
   a refused exec gets, for the same reason `docs/design/sandboxing.md`
   gives it there: a command that visibly did not run cannot honestly
   report otherwise. A diagnostic names the plugin, and an `EventError`
   goes to the sink.
2. **The name keeps failing, and this is a correction.** This document
   said the registrations should be withdrawn, so that the name
   afterwards resolved as if the plugin had never existed. **That cannot
   be done safely and is not done.** `Runner.clone` shares the
   registration map with every subshell — `c := *r` copies `Vars`,
   `Arrays`, `fds` and the traps, and does *not* copy `custom` — so
   calling `Unregister` from a builtin's goroutine writes a map a
   concurrent background job is reading. `cmd1 & cmd2` is enough. That
   is exactly the race `interp.Gate`'s own doc comment warns about, and
   it is worse than the shadowing it would fix.

   So a dead plugin's names go on answering 126 with a diagnostic that
   names the plugin and says it is gone. The objection stands and is not
   dismissed — a dead plugin does shadow a working external command for
   the rest of the run — but it is a wrong answer that says so, against
   a data race that does not. Doing it properly needs either a
   registration table a subshell copies, or a way for a builtin to say
   "not me, resolve onwards", and both are `interp` changes with no
   other caller. Flagged below.
3. **The shell does not die.** A plugin's exit never becomes the shell's
   exit, its stderr never becomes the shell's status, and a malformed
   message ends the *plugin* and not the shell. A decoder panic is
   caught by the panic guard the driver already installs.
4. **No automatic restart.** A plugin that crashed on the input it was
   given will crash on it again, and a restart loop turns one failure
   into a spawn storm. A restarted plugin has also lost whatever state
   earlier calls put in it, so the shell would be talking to a different
   program under the same name without saying so. Flagged below.

### Not parking a goroutine — the rules, and the test that would catch it

Structural, so that the property does not depend on review:

- **Every goroutine the host starts has a reason to return that the host
  controls.** The read loop returns on EOF, and EOF arrives because the
  process is killed — killing is what makes a blocked read return, in
  the same way that unlinking a FIFO does *not* unblock an open already
  in progress.
- **Nothing blocks on an operation whose only unblocking event is a
  cooperating peer.** A plugin that stops reading makes a large write to
  its stdin block indefinitely, which is #690 in a new costume. Writes
  go through a bounded channel that is **abandoned** on shutdown rather
  than drained.
- **Shutdown is: close stdin, wait a bound, `SIGKILL` the group,
  `Wait`.** `Wait` is called exactly once and is what reaps the child. A
  shell that accumulates zombies is worse than one that leaks a
  goroutine, because the process table is shared.
- **The host does not serialize calls, and a plugin that serves one at a
  time must not be put on both ends of one pipeline.** The call id
  exists because two calls into one plugin are ordinary — a background
  job, each half of a pipeline — and serializing them would let a
  background job block a foreground command into the same plugin. The
  consequence is a plugin-author obligation, and it is the sharpest edge
  in the protocol: `plugincmd x | pluginsink` against a single-threaded
  plugin deadlocks, because the sink call asks for input the other call
  has not been reached to produce. Found by writing exactly that test
  against a fixture written in shell. A `concurrent` field at the
  handshake, with the host serializing when it is false, is the obvious
  fix and has no caller yet; flagged below.

- **A call's end has to be waited for, not just recorded, and this is
  the fourth correction.** A host method runs on a goroutine of its own,
  so one sent immediately behind an invoke response is dispatched while
  the builtin is still returning — and if it reaches the runner after
  that, it is touching memory the interpreter has resumed using. The
  race detector found exactly this, against a fixture that sends
  `shell/setVar` behind its own response: a variable set in a shell that
  had moved on. So closing a call marks it closed *and waits* for any
  host method already inside the runner to come out, and liveness is
  checked under the same lock that the waiting takes rather than beside
  it.

  One residual, stated because it is a deliberate trade: `shell/read`
  is not covered by that wait. It holds a lock of its own, precisely so
  that a plugin blocked on the command's input cannot hold up the
  command's return — and a shell a plugin can hang by asking to read
  would be worse than a read that consumes a byte it no longer owns.
  That residual touches no shared memory, so it is a wrong answer
  rather than a race.

- **The host is safe for concurrent use.** A background job and each
  half of a pipeline call from their own goroutines — the same warning
  `interp.Gate` carries, where "the first gate written against this
  package was a closure appending to a slice, and it had a data race the
  moment a script said `&`."

The assertion that makes this gradeable rather than asserted: a test
that runs many plugin lifecycles in one process and requires the
goroutine count to return to its baseline, against fixtures that
misbehave in each specific way — one that hangs at the handshake, one
that exits mid-call, one that stops reading its input, one that writes
garbage on its output, one that ignores a cancel.

## Package layout

    internal/jsonrpc/   JSON-RPC 2.0 over a newline-delimited stream,
                        extracted from internal/acp unchanged
    internal/plugin/    the host
      wire.go           the v1 message shapes, and the transport decision
      host.go           launch, the gate consultation, lifetime, shutdown
      command.go        the command role → interp.Register
      observer.go       the observer role → interp.Sink (not yet)
      testdata/         twelve plugins, every one a POSIX shell script
    cmd/sh              -plugin PATH, repeatable — plugin.go

The extraction is the first thing to do and it is worth doing rather
than copying. `internal/acp`'s framing is already what this needs — a
`Conn` with no side, symmetric, handling concurrent calls — and a copy
of it would be, in this repository's own words about a duplicated
stream resolution, "a place for the decision to go stale". The promotion
rule says a package is promoted once something has consumed it; the
second consumer is what earns the extraction.

`internal/plugin` imports `interp`, `internal/jsonrpc`,
`internal/boundary` and the standard library, and **nothing under
`dialect/`**, asserted by a test over the real
dependency list at any depth — the same test
`internal/policy/dialectblind_test.go` already runs, for the same
reason. A plugin cannot ask which shell it is inside because nothing
tells it, and that stops being a claim about today's code the moment the
test exists.

## What this is not

It is not a sandbox. Approving a plugin approves *starting* it; what it
then does is its own business, and containment is
`docs/design/sandboxing.md` composed with an OS backend.

It is not a plugin manager. There is no index, no install, no update and
no version resolution — those are product-layer concerns, and putting
them in the substrate would be exactly the erosion the opening section
is written against.

It is not a dialect mechanism. A dialect is the three ways; a plugin
cannot set a semantics axis, add a grammar construct, or change a
diagnostic.

## Open, and flagged for the maintainer

Each is a defensible default chosen so the work could proceed, and each
is genuinely the maintainer's to reverse.

1. ~~**JSON-RPC over stdio rather than gRPC.**~~ **Decided, 2026-09-06:
   "lets go with JSON-RPC we can go with gprc later."** The maintainer
   asked for gRPC when he opened #738 and changed direction after
   reading the argument above. The issue title still says gRPC and is
   not being rewritten.

   What "later" means is written down in `internal/plugin`'s package
   comment rather than only here, because the risk is real: **no
   abstraction has been built to accommodate a second transport, and
   none should be until a plugin needs one.** Two transports invented
   before either has a plugin would be shaped by neither, which is
   #801's warning one layer up. What would have to be true to add one is
   a real plugin needing typed streaming or generated stubs, or a
   measured cost this encoding is paying — and the argument gets made
   then, with a caller in hand.
2. **Gate consultations are excluded from the plugin surface.** The
   decision most likely to be wrong. The obvious narrower compromise is
   `ActionExec` only — rare enough for a round trip, and already the
   escalation set `docs/design/acp.md` chose for a person.
3. **Stream chunks are base64 in JSON.** The one place gRPC would be
   plainly better. Passing real descriptors instead needs `SCM_RIGHTS`
   over a Unix socket, which is Unix-only and awkward from several
   languages.
4. **No automatic restart of a dead plugin.**
5. **A plugin's stderr is relayed to the shell's stderr with a prefix**
   rather than discarded or routed to the sink.
6. **A plugin may claim a name a core builtin already has.** `Register`
   permits replacement, and the conservation rule says the plugin
   surface is `Register`'s surface. Names are declared at the handshake,
   so it is at least reviewable.
7. **Registrations are fixed for the plugin's life.**
8. **A plugin that fails to launch is fatal to the invocation**, status
   2, rather than a warning.
9. **Completions are excluded** while completion is on the keystroke
   path, and the in-process seam landing first (`repl.Completer`) is
   what makes the remaining objection a statable one.
10. **Plugins start eagerly at startup** rather than on first use.
11. **A dead plugin's names keep failing rather than being withdrawn.**
    Not a preference — see the lifetime section, where the withdrawal
    this document originally specified turns out to be a data race
    against every subshell. Fixing it properly is an `interp` change
    with no other caller today.
12. **The host does not serialize calls into one plugin**, so a plugin
    that serves one at a time cannot be used on both ends of a
    pipeline. A `concurrent` field at the handshake would let the host
    serialize for the plugins that want it; it has no caller yet.
13. **A plugin inherits the process's environment**, which is a snapshot
    from the moment of the launch and is not the shell's variables.
    `shell/getVar` is the live answer. Handing it a filtered
    environment, or none, is the alternative.

## Staging

1. **Done** (#740) — the design, this document;
2. **Done** (#1017) — `internal/jsonrpc`, extracted from `internal/acp`
   with no behavior change and `internal/acp` rewritten onto it;
3. **Done** — `internal/plugin`: launch, the gate consultation, the
   handshake, shutdown, and the lifetime tests;
4. **Done** — the command role, and `cmd/sh -plugin`;
5. the observer role — `interp.Sink` remoted, as bounded-buffer
   notifications that drop rather than block, which `seq` makes honest;
6. ~~a reference plugin in a language that is not Go~~ — **overtaken**.
   Every fixture in `internal/plugin/testdata` and the plugin in
   `cmd/sh/plugin_test.go` is a POSIX shell script, chosen for exactly
   this reason: a fixture written in Go would test the host against a
   peer built from the same message types, which is the one peer that
   cannot check the "any language" claim. What is still owed is a
   *documented, published* example rather than a test fixture, and per
   the maintainer's "maybe a plugins repo so that they can be used" it
   belongs outside this repository.

Steps 3 and 4 landed together, because a host with no role has no
in-tree caller — the rule #804 applied when it shipped three seams and
declined the fourth. `Closes #738` waits on step 5, which is the last
thing in this document that is not yet true.
