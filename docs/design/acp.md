# ACP: an Agent Client Protocol front end, both ways round

The gate and the event stream are the shell's native permission-and-audit
surface, and they were built for three consumers at once: a sandbox, an
AI assistant, and an agent protocol. This is the design for the third.

Read `docs/design.md` first. The claim this document depends on is the
one made there — that a policy enforced outside the interpreter does not
reach inside it — because it is what makes an agent front end honest.
An editor that spawns `sh -c` and watches the pipe sees what a script
chose to print. An ACP front end over this seam sees every exec, every
open, every stat and every signal, including the ones inside an `eval`.

## Two directions, one protocol

ACP has two roles. An **Agent** does work and asks permission; a
**Client** provides the environment, answers, and shows a person what is
happening. This shell is both, in two different arrangements, and the
whole point of the design is that it is one implementation rather than
two:

    editor  ──▶  sh --acp                       sh is the Agent
    sh      ──▶  claude / gemini / codex        sh is the Client

**Agent side.** An editor launches `sh --acp` as a subprocess and sends
it shell to run. Gate consultations become permission requests to the
editor; the event stream becomes session updates; execution goes through
the existing driver routes. This is a fourth consumer of `driver.Shell`,
beside `-c`, a script file and the prompt.

**Client side.** The shell launches a coding agent as a subprocess and
is the environment that agent runs inside. The agent asks *us* for
permission, asks *us* to read and write files, asks *us* to run
commands — and every one of those requests goes through the same gate.
That is the part worth stating plainly: **an ACP client that is a shell
can enforce a policy on an agent that no editor can**, because the file
the agent reads is a file *we* open, through the boundary, and the
command it runs is a command *we* start, through the boundary.

The two roles are not a fork of each other. What follows is written as:
the protocol facts, then the shared core, then what is role-specific,
then the compatibility surface of the three agents we must talk to.

## Which revision, which transport, and how that was established

**Protocol version 1**, as published by the Agent Client Protocol
project, wire schema `schema/v1` at release 1.21.0 (2026-08-20).
`protocolVersion` is the integer `1`; it is bumped only for breaking
changes, and everything else is negotiated through capabilities.

**stdio**: the client launches the agent as a subprocess, and JSON-RPC
2.0 messages cross on the agent's standard input and standard output,
UTF-8, one message per line, with no embedded newlines. The agent may
write logs to standard error and **must not** write anything to standard
output that is not an ACP message. Both halves of that rule bind us,
once each way round.

Every message shape in this document was taken from the machine-readable
schema rather than from prose: `schema/v1/schema.json` (170 definitions,
each carrying `x-side` and `x-method`) and `schema/v1/meta.json`, which
is the method table. The narrative pages —
`docs/protocol/v1/{transports,initialization,prompt-turn,tool-calls}` —
were read for the rules a JSON schema cannot express, such as which
party must answer a pending permission request when a turn is cancelled.
Nothing here was inferred from an SDK's source.

A version 2 exists and is **not** targeted. It is `2.0.0-alpha.3`, it
reshapes authentication and drops the `fs/*` and `terminal/*` client
surface in favour of a different one, and shipping against an alpha
would mean re-cutting the mapping when it moves. Every agent we have
measured speaks version 1. The wire types live in one file for exactly
this reason: when v2 stabilizes, the mapping below is unchanged and only
the encoding moves.

## What is shared, and what is role-specific

This is the table the implementation is organized around. "Shared" means
one implementation serves both directions; a design in which the client
side re-implements any of the first column is the failure to avoid.

| concern | shared | role-specific |
| --- | --- | --- |
| JSON-RPC framing, ids, concurrency | **all of it** — a `Conn` is a peer, not a side | which methods it is given to answer |
| message shapes | **all of it** — one party writes what the other reads | — |
| the capability handshake | the negotiation, the version check, the refusal | which capabilities are ours to claim |
| the permission option model | **all of it** — the four kinds, the ids, the meaning | who asks and who answers |
| the session model | id, working directory, one turn at a time, cancellation | what a turn *is* |
| gate and event plumbing | **all of it** — same `interp.Gate`, same `interp.Sink` | which actions reach it |
| the shell itself | `driver.Session`, one per ACP session | — |

Two things fall out of that table and are worth naming because they are
easy to get wrong.

**A `Conn` has no side.** JSON-RPC is symmetric: both parties answer
requests and both make them. The transport is written once, takes a
handler, and can call out while a handler is running — which is not an
optimization but a requirement in *both* directions. An agent answers
`session/prompt` by calling back with `session/request_permission`; a
client answers `terminal/create` by calling back with nothing yet but
will, and in the meantime it is still receiving `session/update`.

**The permission mapping is direction-independent.** The four
`PermissionOptionKind` values mean the same thing whoever is looking at
them: two of them allow, two refuse, and one of each remembers. Turning
an option id into an `interp.Decision` is the same function whether we
chose the option (client side, from a person) or are asking somebody
else to choose it (agent side).

## The shell as Agent

An editor spawns `sh --acp`, sends it a command, watches what the shell
does, and answers when the shell asks whether it may.

### The subset a shell meaningfully serves

ACP is written for agents that talk to a model. A shell has no model, no
plan and no token budget, and the honest subset is smaller than the
protocol. Implemented: `initialize`, `session/new`, `session/prompt`,
`session/cancel`; outbound, `session/update` and
`session/request_permission`.

Refused with `-32601`, each for a reason:

- **`authenticate` / `logout`.** `authMethods` is empty. A local shell
  authenticates by being a process the user already started.
- **`session/load`, `session/resume`, `session/list`.** `loadSession` is
  advertised false. A shell's session state is its variables, functions,
  working directory and descriptor table, and none of that is in the
  update stream: restoring the transcript would restore the *appearance*
  of a session.
- **`session/set_mode`, `session/set_config_option`.** The obvious
  candidate is the dialect, and it is not free: a dialect is runtime
  state that `set -o posix` re-reads mid-script, so exposing it as a
  config option needs an update every time a script changes it. Worth
  doing on purpose rather than by accident.
- **MCP servers.** `session/new` carries `mcpServers`; ours is accepted
  and ignored. A shell is not an MCP host.

**`fs/*` and `terminal/*` are not called even where the client
advertises them.** A shell reads files by opening them, through the
gate, in the interpreter. Routing a `cat` through the client's file
system capability would move that read *outside* the boundary this whole
seam exists to draw, and the audit trail would lose it.

### What a prompt is, when the agent is a shell

The text blocks of `session/prompt`, joined with newlines, are the shell
program for the turn. It runs on the session's `driver.Session`, which
is the `-c` shape: the origin is labeled `-c` in a parse failure's
location, `$0` is the shell, and there are no positional parameters.
Non-text blocks are accepted and skipped, because the baseline requires
an agent to tolerate a resource link and there is nothing useful a shell
does with one.

`stopReason` is `end_turn` when the program finished, whatever its exit
status — a failing command is a turn that completed — and `cancelled`
when it was interrupted. `refusal` is reserved for a program that could
not be parsed at all. `max_tokens` and `max_turn_requests` have no
meaning here and are never sent.

**One turn at a time per session.** A second `session/prompt` while one
is in flight is refused with `-32602`: the runner is a single shell, and
interleaving two programs in it would give neither the variables it
wrote. `driver.Session` says the same thing in its own words.

### The three streams

An ACP agent may not write to standard output anything that is not a
protocol message, and a shell's whole job is writing to standard output.
The resolution is that the shell's stdout and stderr are not the
process's: `driver.Shell` takes them as `io.Writer` fields, so the
session hands the runner writers of its own and each write becomes a
`session/update` — `agent_message_chunk`, with the stream named in
`_meta`, because v1 has no update kind that means "diagnostics" and
`agent_thought_chunk` means a model's reasoning.

Standard input is empty; a `read` gets end of file. That is the
deliberate answer to the interactive question below.

It is `os.DevNull` and not an in-process empty reader, which is a choice
rather than a leftover now that `driver.Shell.Stdin` is an `io.Reader`
(#787). A session's shell hands its standard input to every external
command it runs, and `os/exec` connects a child straight to an `*os.File`
and builds a pipe for anything else. A file costs the session one
descriptor; a reader costs every command in it a pipe and a copying
goroutine.

## The shell as Client

The shell launches an agent and provides its world. This is the half
where the gate does something no editor's gate can.

### What a client must implement

- **`session/update`** — receive and render. This is the agent telling
  us what it is doing.
- **`session/request_permission`** — the agent asking. Somebody has to
  answer, and the answer is the same mapping the agent side produces,
  read the other way round.
- **`fs/read_text_file` / `fs/write_text_file`** — optional by the
  protocol, and the reason to implement them is the whole argument
  above: a file the agent asks *us* to read is a file that passes our
  gate and lands in our audit trail, where a file the agent opens for
  itself is invisible to us. Advertising them is how we pull the agent's
  file access inside the boundary.
- **`terminal/*`** — the same argument for commands, and additionally
  required for one agent's authentication (below).

### Authenticating, and the two kinds that are not two spellings

Authentication is the **first** thing that happens on this side, not the last,
because two of the three agents refuse `session/new` with `-32000` until it
has. `sh -acp-connect` therefore settles it between `initialize` and
`session/new`, and `-acp-auth ID` names which of the advertised methods to use.

**There is no default.** A client that picked a credential path on somebody's
behalf would be guessing at their account, so a run without the flag attempts
nothing and reports the list — with each method's kind, because the kind is
what decides what a person has to *do* — and names the flag.

The union is discriminated on `type`, and **an absent `type` means `agent`**:
that is the schema's default rather than an unknown, and it is not academic —
all four of Gemini's arrive with no `type` at all, so a client dispatching on
the raw field would find `""` and match neither kind.

| kind | what the client does |
| --- | --- |
| `agent` | calls `authenticate` with the method id and waits |
| `terminal` | **does not call `authenticate` at all** |

Terminal Auth is the one a shell is unusually well placed to serve, and it is
not a message. The client runs the **agent's own program again** — same command,
same arguments, plus the method's `args`, with its `env` set over the top — on a
terminal a person can type into, and a zero exit status is the whole of the
answer. The schema states plainly that a terminal method must never be passed
to `authenticate`, so the two are dispatched on the kind rather than tried in
turn.

Three things follow, and each is a rule rather than a preference:

1. **The capability and the ability to honor it are one field.** `internal/acp`
   does not know how the agent was launched — only the caller that launched it
   can reproduce that invocation — so the relaunch is a hook, and it is the
   hook's presence that sets `clientCapabilities.auth.terminal`. There is no
   arrangement in which an agent is offered a login this client cannot run.
2. **`sh -acp-connect` offers it only where the process has a terminal**, on
   both standard input and standard output. A login TUI reads keystrokes *and*
   draws, and `sh -acp-connect … < script` has a terminal on exactly one of the
   two.
3. **A method the agent did not advertise is refused here**, without a message
   being sent. It is the same rule as an option id we never offered, read the
   other way round: the agent named the choices, so a choice it did not name is
   not one.

Measured against Gemini CLI 0.58.0, `authenticate` with `gemini-api-key` is
answered `{"result":{}}` and `session/new` then still answers `-32000` —
because the machine has no Gemini credential, which is a human action rather
than code. The end-to-end verification is **#729**; the client-side method is
this.

The shapes for all of this were taken from the machine-readable schema shipped
with `@agentclientprotocol/sdk`, which is where `AuthMethodTerminal`,
`AuthMethodAgent`, `AuthCapabilities` and `AuthenticateRequest` are defined,
rather than from prose or from any SDK's source.

### `terminal/*`, and what the gate can see because of it

An agent that cannot ask a client to run something runs it itself: its own
fork, its own exec, its own argv, and **no gate anywhere sees it**. That is not
a gap at the edge of the design — it is the whole of it. A shell's policy on a
coding agent that cannot see the commands the agent runs is a policy on the
agent's file reads and nothing else.

So the five methods are implemented and advertised, for the same reason `fs/*`
is and one step further along the same argument:

| inbound | what happens |
| --- | --- |
| `terminal/create` | with `args`: `ActionExec` with the argv, through the gate, then `exec.Cmd`. Without: the line is interpreted by this shell — see below |
| `terminal/output` | what it has written so far, and its exit status if it has one |
| `terminal/wait_for_exit` | blocks until it ends, or until the request's context does |
| `terminal/kill` | `ActionSignal` through the gate, then `SIGKILL` |
| `terminal/release` | recorded, not gated — see below |

**A refused `terminal/create` is a command that never started**, and the agent
is told so as an error rather than an empty terminal, because a terminal id
that names nothing is a thing it would then poll.

**`args` is what picks between two readings of `command`, and both are
served.** The field is optional in the schema, so an agent that sends one has
built an argv and wants it exec'd, and an agent that sends only `command` has
written a line for a shell. Taking the first reading for both is what #1782
was: `claude-code-acp` 0.16.2 sends `printf "argv0=%s\n" "$0"; ps -o args= -p
$$` and no `args`, that arrived at `exec.CommandContext` as a program name,
and nothing ran. Its own workaround — sending `bash -c '…'` — fails the same
way, because that is one string as well.

For this client the second reading is not a concession, it is the argument of
this whole document one step further. **A line we interpret ourselves is a
line whose every `exec`, `open` and `stat` crosses the boundary and lands in
the audit trail.** Exec'ing it as a filename is the reading that both fails
and sees least: one gate consultation about a name that never existed.

So `createInterpreted` deliberately takes **no gate consultation of its own**.
There is no program at that moment to ask about — the line may run none, or
ten — and what replaces the question is strictly more than it: a policy that
refuses `/bin/rm` still refuses `rm -rf /` sent as a line, and the record now
names `rm` rather than a file that was never there.

The two readings run different things behind one terminal id, which is what a
`vehicle` is for. A child process is ended by signal and reports a wait
status; an interpreted line has no process to signal, ends through its
context, and reports a code. `terminal/kill` is gated in both readings — an
interpreted line names pid 0, which the audit schema already admits — and
`terminal/release` records a signal only where there was a process to name,
because a `SIGKILL` against pid 0 would be a fiction.

**Release is recorded and not gated, and kill is gated.** They both end a
process, so the difference has to be argued rather than assumed. `terminal/kill`
is the agent reaching a running process it chose to reach, which is exactly the
`ActionSignal` case. `terminal/release` is the protocol's only way to say "I am
finished with this", and the signal inside it is *the client ending something
the client started*, on the same rule `internal/boundary` already draws: an
access is inside the boundary when the path — here the process — was chosen by
whoever the policy is about. Refusing a release would also leave this client
holding the process forever, with the agent given no other way out.

**Both of the command's streams become one.** A terminal has one, an agent
asking for output is asking what a person would have seen on a screen, and
splitting them would invent a distinction the protocol does not have.

**`outputByteLimit` truncates from the front, at a character boundary**, which
the protocol requires in so many words. The retained slice is copied rather
than resliced: a reslice leaves the dropped prefix alive in the array
underneath, so a long-running command would hold every byte it ever wrote,
which is the one thing a limit exists to prevent.

**A terminal outlives the request that made it and dies with the connection.**
The context a handler is given is the connection's, which is the right lifetime
for a process the agent will come back to — and it means an agent that
disconnects mid-command does not leave one running.

The environment is inherited and then written over rather than replaced. A
command started with only the agent's few variables has no `PATH`, so every
`terminal/create` would fail for a reason nothing on the wire explains.

#### One of the two measured agents declines the route, and what is done about it

**The earlier measurement here was wrong, and it was our bug that made it
wrong.** It read: "Claude Agent 0.75.1 and Codex 1.10.0 both ran it in their own
process and called no client method at all." That was taken while #1777 was
live — this client answered every permission request with an option id the agent
had never offered, so the agent was being told its tool call was **rejected**
and was falling back to doing the work itself. What was written down as an agent
declining the honest route was an agent being refused and working around it. A
measurement of a client's own defect, recorded as a fact about somebody else.

Re-measured 2026-09-10 with the fix in, through `sh -acp-connect`, each agent
asked in the same words, with `terminal: true` and both file methods
advertised:

| asked | Claude Code 0.16.2 | Codex 1.11.0 |
| --- | --- | --- |
| "run `echo hello-from-acp`" | **asked us**: `terminal/create`, gated and traced | ran it itself; no client method; gate saw nothing |
| "read this file and tell me what it contains" | read it with its own tool; no client method; gate saw nothing | read it with its own tool; no client method |
| "use your file-reading tool, not a shell command" | read it with its own tool; no client method; gate saw nothing | *(not asked)* |
| "create a file containing …", under `-deny write:/**` | wrote it with its own tool; **the file appeared**; gate saw nothing | wrote it itself; **the file appeared**; gate saw nothing; **never asked permission either** |

So the two agents differ, and the difference is the whole question. Claude Code
takes the `terminal/*` route for **commands** — the one thing the old row said
no adapter would do. Codex still does not. Neither takes it for **files**: not
one measured turn, from either agent, called `fs/read_text_file` or
`fs/write_text_file`, and asking Claude in as many words to use its file-reading
tool rather than a shell command did not change that.

That last point deserves to be stated on its own, because this document has
argued the other way round: **the `fs/*` half of this client has never been
exercised by a real agent.** It is tested, and it is correct as far as its tests
reach, and no adapter measured here has ever called it. A policy's reach over an
agent's file access is, today, a reach over something nobody asks for.

The `-deny write:/**` row is the sharpest, and it survives re-measurement with
permission genuinely granted: the file appeared, the gate recorded nothing, and
`-trace-events` printed not one line for the whole turn. The only thing that was
consulted was the *person* — this client answered the agent's
`session/request_permission` with allow — and a person answering a question is
not a policy covering an action.

One honest caveat on the Claude command row: it called `terminal/create` four
times in that turn, not once, because #1782 made every one of them fail and it
retried with a differently-quoted command each time. The route is what is being
measured and the route is real; the count was an artifact of a bug of ours and
must not be read as anything about the agent. That bug is fixed — a bare
`command` is now interpreted rather than exec'd as a filename — so a re-measure
should see one call, and the retries are the thing to watch to confirm it.

So the earlier wording — that a policy covers the agent's file access and not
its commands — is true about what an agent *asks for* and misleading about what
a person gets. A command an agent runs itself is also how it reads and writes,
so a turn can be covered by nothing at all and look exactly like a turn that
was covered by everything. That is what the per-turn notice exists for, and it
is why it names files as well as commands.

That is #786, and **nothing in this repository can close it**: an agent that
forks its own process is outside our boundary by construction, exactly as any
allowed `exec` is once it has started. Serving `terminal/*` is what makes the
honest route *exist*; whether an agent takes it is the agent's.

Two things follow, and both are done.

**The claim is stated with its limit, wherever the claim is made.** A policy on
an agent reaches what the agent asks this shell for. `sh -h` says so beside the
flag; so does this document; and the honest version is not "under the same
policy" full stop.

**A person is told, per turn, which half they got.** `Client.Commands` reports
two counts — `terminal/create` requests served, and tool calls of kind
`execute` the agent announced — and `-acp-connect` prints a notice after any
turn where the second exceeds the first. **Neither number is inferred from the
other, and they are not joined**: no id relates an agent's tool call to a
terminal it asked us for, and #719 already declined to invent one. The counts
are reported; the reader draws the conclusion. A turn where the agent asked for
everything it ran says nothing, because a notice that fires on a clean run is a
notice people learn to skip.

**And the route that does work is guarded rather than remembered.** Nine
inbound methods reach the world through `internal/boundary` today, each by a
hand-written call. The tenth will be written by copying one of them, and a copy
that drops the boundary call still compiles and still passes every test about
the other nine — so `TestEveryInboundMethodDeclaresWhatItDoesAboutTheBoundary`
reads `Client.Handle` and fails on a `case` that is not declared as gated,
recorded or touching nothing outside this process. The bypass that *can* be
reintroduced here is the one by omission, and that is the one closed.

### Where the gate sits on this side

Every inbound request that would touch the world is an `interp.Action`
before it is anything else:

| inbound | action asked of the gate |
| --- | --- |
| `fs/read_text_file` | `ActionOpen`, `Write: false` |
| `fs/write_text_file` | `ActionOpen`, `Write: true` |
| `terminal/create` | `ActionExec` with the argv, or — for a bare command — whatever the interpreted line itself does |
| `terminal/kill` | `ActionSignal` |

A denial is answered as the protocol allows an error to be answered, and
recorded through the same `Sink`. The shape of the boundary is
unchanged: `internal/boundary` already exists for exactly this — a front
end asking the interpreter's gate about the front end's own accesses —
and this is a fourth caller of it rather than a new mechanism.

`session/request_permission` from the agent is the case where the gate
is *not* the whole answer: the agent is asking about something it will
do itself, in its own process, which our boundary does not cover. We
show it to the person and send back what they chose. Where our own
policy has already refused the underlying access, we may answer without
asking, which is the composition rule below applied in the other
direction.

## The gate and the permission model

### Which actions reach a person

Not all of them, and this is a judgement rather than a reading of the
protocol. A single `ls | grep x` stats every PATH entry it tries; a glob
stats and reads directories in bulk. A permission prompt arriving at
that rate is a prompt people click through, which is a *worse* boundary
than an honest record, because it converts a considered answer into a
reflex.

The escalation set defaults to the actions that change something outside
the shell or start something the boundary can no longer see:
`ActionExec` always, `ActionOpen` with `Write`, and `ActionSignal`.
Reads — a non-writing open, a stat, a directory read — are allowed and
**recorded**, never asked about. `ActionInherit` is not gated at all, by
the interpreter's own contract.

The set is a value rather than a constant, so a caller that wants
everything escalated can have it. What it is not is a policy language:
refusing reads by rule is `docs/design/sandboxing.md`, and this composes
with that rather than duplicating it.

### Composition with a real policy

The ACP gate wraps an inner `interp.Gate`, which is whatever gate the
shell already had — `-policy`, `-deny`, or an embedder's own:

1. the inner gate is consulted first; if it denies, the action is denied
   and **no one is asked**. A policy refusal is not negotiable, and
   offering a person a button that overrides the sandbox would make the
   sandbox advisory;
2. otherwise, if the action is in the escalation set and no remembered
   answer covers it, the person is asked;
3. otherwise it proceeds.

This is why no third `Ask` decision is needed in `interp`. The
interpreter's question stays binary; *who answers it* is the front end's
arrangement.

**The wiring is the part that was missing, and for a while it was.** The
rule above was written and unit-tested before there was a policy to put
inside it, and `session/new` then assigned its gate *over* the
template's rather than around it — so `sh -acp -policy p` took the flag
and consulted the rules nowhere. Measured on `main` at the time (#1335):
a policy of `default deny exec` plus `allow exec /bin/echo` refused
`/usr/bin/curl` under `-c`, and ran it under `-acp` as soon as the
client answered `allow-once`. The tell was not that curl ran. It was
that permission was **requested at all**, with a policy present and
without one alike: the question was the same either way, which is what
"the policy is not being asked" looks like from the outside.

Two things follow for anything built on this seam. The first is that a
test about this route asserts the request **does not happen**, not that
the command was refused — a client answering reject makes a broken shell
look correct. The second is why that matters more here than at a prompt:
the far end of `-acp` is not necessarily a person. An agent harness
answers permission requests itself, and one configured to auto-approve
turns every escalation into an allow, so a policy that is merely *asked
about* is a policy that is gone, with nobody in a position to notice.

### The event sink composes the same way

A session's events go to the client **and** to whatever sink the
invocation installed. `-audit f` writes a file and `-trace-events`
writes to the terminal, and somebody who served ACP from a shell that
was already keeping a record did not ask for the record to stop: an
audit trail a front end can silence is not an audit trail. This was the
same assignment-instead-of-composition mistake, on the line below the
gate, with the same shape — `sh -acp -audit f` created the file and
wrote nothing to it — and it is fixed with the same rule `cmd/sh`
already applies to a plugin's observer.

### The four option kinds

| optionId | name | kind | effect |
| --- | --- | --- | --- |
| `allow-once` | Allow | `allow_once` | `interp.Allow`, remembered nowhere |
| `allow-always` | Allow always | `allow_always` | `interp.Allow`, remembered |
| `reject-once` | Reject | `reject_once` | `interp.Deny` |
| `reject-always` | Reject always | `reject_always` | `interp.Deny`, remembered |

An unrecognized `optionId` in a response is a **deny**, not a guess.

**What "always" remembers** is the pair (action kind, path) — for an
exec the resolved program, for an open the file and whether it was for
writing, for a signal the target pid, which is deliberately near-useless
because a pid is not a stable identity. The memory lives on the
**session**, not the connection: sessions have their own working
directory and a client presents them as separate things.

It is an exact match and not a prefix. "Allow always for everything
under /tmp" is a policy language, and writing one here would be the
sandboxing initiative done badly in the wrong package.

### Deny until answered

Every path that is not an explicit allow is a denial: outcome
`cancelled` (which a client **must** send for pending permission
requests when a turn is cancelled), a reject kind, an option id we never
offered, a JSON-RPC error, the connection closing with the request
outstanding, or the turn's context being cancelled while waiting.

There is **no timeout**. A client that never answers holds the
goroutine, and that is correct: a deadline that silently allows or
silently refuses is a boundary that reports something other than what
happened. Cancellation is the way out, and it is a message the protocol
already has.

### Asking a person, and where the question can live

Permission requests on the client side were settled by `-acp-allow` — allow
everything or refuse everything, chosen at the command line. That is a
placeholder for a person and a deliberately bad one; this replaces it where a
person is actually reachable.

**The ladder, in the order it is preferred.** The last one is the default and
has to be:

1. `-acp-allow` answers allow-once to everything without asking. The question
   and the answer still go to standard error, so a run made this way leaves the
   record a person would have been shown.
2. A terminal on both standard input and standard output: **the person is
   asked**, and their answer is one of the four options the agent offered.
3. Neither: reject-once to everything. **Nobody to ask is a denial**, which is
   the rule the whole permission model is built on, unchanged.

An answer that is not one of the offered option ids is a refusal, and so is the
input ending with the question outstanding. There is still no timeout.

**The reader is the prompt loop's own**, shared rather than duplicated, and
that is safe by the shape of a turn rather than by luck: `talk` reads a line,
hands it to `session/prompt`, and blocks there until the turn ends. A
permission request or an elicitation only arrives *during* a turn, so the
scanner is idle exactly when a question needs it. Two readers on one descriptor
would race for bytes and lose lines to whichever won.

It is a line read rather than `repl`'s line editor, which is worth stating
because this shell has one. What `repl` exports is `Shell.Run`: a whole prompt
loop that owns the terminal, the history and the shell it drives. There is no
single-line entry point to borrow, and taking the terminal into raw mode for a
one-word answer, in the middle of a turn whose output is still arriving on the
same screen, would be a worse answer than a plain read rather than a better
one. If a line editor is ever wanted here, the thing to add is a read-one-line
entry point to `repl`, not a second editor.

#### `elicitation/create`

A permission request is not the only thing an agent may need a person for, and
until v1 grew `elicitation/create` it was the only one the protocol had. An
agent that needs a choice, a name or a confirmation asks for it there, and this
client serves it at the same terminal, one line per field.

**Form mode only**, and it is advertised as only that. The schema advertises
each mode by supplying `{}` for it, so a client that cannot draw a form or
cannot open a browser simply does not name that mode — and the rule the file
and terminal capabilities are already held to applies again: a mode that is
served is a mode that was claimed, and a mode that arrives unclaimed is
refused rather than guessed at. URL mode would mean opening a browser and
waiting for an `elicitation/complete` notification, which is a different
mechanism and not one a terminal answers.

Within a form, the primitive property types are asked for and coerced: a
string, a number, an integer, a boolean, and the single-select enum, which the
schema spells as a string property carrying `enum`. The multi-select case is an
array property and is **not** served: a terminal line is a poor multi-select,
and offering a bad one is worse than saying so.

A required field left empty declines the whole elicitation, because a form
returned without what it required is not an answer to it. Declining and
cancelling are answers too — the agent is owed one either way — and, as with a
permission request, the input ending is a decline rather than a hang.

#### Why the agent side still cannot ask

The **agent** side of this shell remains strictly non-interactive, and
`elicitation/create` does not change that today, although it is the right route
eventually.

The reason is mechanical rather than philosophical, and it has moved once.

**It used to be the field.** A session's standard input is `os.DevNull`, so a
script's `read` gets end of file; making it reach a person means making that
reader *demand-driven* — the question is only worth asking when a script
actually reads. `driver.Shell.Stdin` was an `*os.File`, so there was nowhere to
put a reader that calls out over the connection: an `os.Pipe` fed by a
goroutine cannot know when somebody reads the other end, so it would have to
elicit eagerly, which asks a person a question no script ever asked.

**That field is now an `io.Reader`** — #787, done on purpose rather than as a
side effect of this, with the terminal checks and `repl.Shell.In` widened
alongside it. A reader that performs a round trip fits the field. It is still
not enough, and the obstacle one level down was measured rather than guessed:

> **A shell's standard input is inherited by every external command it runs,
> and `os/exec` reads a non-file one on the child's behalf whether or not the
> child ever reads it.** It connects a child directly to an `*os.File` and
> builds a pipe for anything else, filled by a copying goroutine that starts
> when the command does. `/bin/echo hi`, which reads nothing, causes one
> `Read`.

So a reader that asks a person would be asked **once per external command**,
not once per `read` — which is the eager question the original argument was
against, arriving through a different door. The tripwire for it is
`TestAReaderIsReadOnAChildsBehalfWhetherOrNotTheChildReads` in `driver`: if
`os/exec` ever stops copying an untouched stdin, that test goes red and this
paragraph is out of date.

**Two routes are open and neither is taken here**, because both are somebody
else's decision to make:

1. **Let interp say what a child inherits.** The eliciting reader would be the
   shell's own input and a child would keep `os.DevNull`, which is what every
   child in a session gets today — so nothing regresses and `read` gains a
   person. It needs a field on `interp.Runner`, and a second meaning for "the
   shell's input" is a change to what a shell *is*, not a plumbing detail.
2. **Elicit from the `read` builtin rather than from the stream.** Narrower and
   more honest about what is being asked — a `read` is the only thing in a
   shell that wants a line from a person — but it is a seam in `interp` whose
   only caller would be this package, which is the shape this repository
   already declines to build on speculation.

Until one of them is chosen: **a script's `read` under `sh -acp` gets end of
file, and the agent side does not ask.** Widening the field was necessary and
is not sufficient, and that is the whole of the change in this paragraph.

## The event stream becomes session updates

| event | update |
| --- | --- |
| `EventCommandStart` | `tool_call`, kind `execute`, status `in_progress` |
| `EventCommandEnd` | `tool_call_update`, `completed` or `failed`, exit status in `rawOutput` |
| `EventDenied` | `tool_call_update`, `failed` (or a `tool_call` if none was open) |
| `EventError` | `tool_call_update`, `failed`, message as content |
| `EventAccess` | `tool_call_update` content on the open command, or a standalone `tool_call` of kind `read` / `edit` / `other` |

A tool call's `title` is the command line as written, its `locations`
carry the path for a file action so the client can follow along, and its
`rawInput` carries the structured action.

An escalated action creates its tool call *before* the permission
request, because `session/request_permission` requires a `toolCall`. The
gate runs before `EventCommandStart`, so the tool call exists by the
time the event arrives and the event updates it rather than creating a
second.

### Correlation: one id, asked for rather than invented

`interp.Event` used to carry no identity for the action it belonged to, so a
start and an end were matched by *ordering* — and ordering is exactly what
concurrency breaks, since a background job and each half of a pipeline emit
from their own goroutines. This front end matched on a fingerprint of the
action instead: kind, path, argv, and the write flag or the signal target,
with a queue of open tool calls per fingerprint. That was right whenever two
identical commands were not in flight at once, and marked the wrong tool call
complete when they were.

It is closed. **#719** — landed as #730 — gave `interp.Action` an `ID` and
`interp.Event` a `Session`, and the id is the same string on the Action a gate
is consulted about and on every event that action produces. That is exactly
the promise this needed, so the fingerprint is gone and the tracker is a set
keyed on the id.

Two things about the shape are worth keeping.

**The tool call id and the action id are the same string.** `ToolCallId` is
required by the protocol and was minted here as `call-1`, `call-2`; there is no
reason for it to be a different value from the one the interpreter already
uses, and every reason for it not to be — a client's transcript and the audit
stream now join on a value both already carry, rather than on a fingerprint
that agrees by luck. Nothing was invented for this: asking for one field
rather than minting a second scheme was the whole argument, because two id
schemes that do not agree are worse than none.

**An action with no id is matched to nothing, not to everything.** Nothing in
the interpreter produces one — an empty id means a Runner with neither a gate
nor a sink, which emits no events either — and a defensive fallback that keyed
the empty string would put every such action in one bucket and close the wrong
tool call, which is the fingerprint's failure brought back by a default. It
still gets a tool call id, because the protocol requires one; it simply joins
to nothing, and an unmatched end becomes a standalone completed tool call,
which is visible.

### Reading the event schema, not only writing to it

The schema's stability rules bind a consumer as much as a producer, and
this side honours them: an event kind this shell does not know is
**reported** rather than dropped, which is rule four — a consumer must
not fail on a name it has not seen, and the useful default is to record
it and carry on. `ActionSignal` arriving after the other five is the
worked example; a mapping that ignored what it did not recognize would
have shown a client a shell that never signaled anything. Zero is never
read as absent, either: a signal's pid and number and a command's exit
status are reported even when they are zero, because zero means
something in all three.

Nothing here reads `seq`. A client sees updates in the order they were
sent on the wire, which the framing guarantees by handling notifications
on the read loop; `seq` is emission order for gap detection and is not a
sequence anything should be presented in.

### Output reaches a client through a pipe, and that is a real cost

The session's writers are not `*os.File`, so `os/exec` builds a pipe for
every child and copies through it. Here that is the mechanism rather
than an accident — it is *how* a command's output becomes session
updates — but it is the same wall the blocks work met (#720), and it has
the same two consequences: a copy per command, and a child that can tell
it is not on a terminal. Programs that colour their output or draw
progress will behave as they do in a pipe. The honest fix is a pty
rather than a workaround, and it is not in this design.

## The three agents we must talk to

All three speak protocol version 1, all three are launched with `npx`,
and there the similarity stops. Measured from the ACP registry's own
daily protocol matrix (`.protocol-matrix/latest.json`, run 2026-09-05)
and the registry's agent entries:

| | Claude Agent | Codex | Gemini CLI |
| --- | --- | --- | --- |
| package | `@agentclientprotocol/claude-agent-acp` | `@agentclientprotocol/codex-acp` | `@google/gemini-cli` |
| version measured | 0.75.0 | 1.10.0 | 0.58.0 |
| ACP by | adapter over the Claude Agent SDK | adapter | **native** |
| launch args | *(none)* | *(none)* | `--acp` |
| `protocolVersion` | 1 | 1 | 1 |
| auth method kind | `terminal` | `agent` | `agent` |
| `session/new` unauthenticated | succeeds | `-32000` auth required | `-32000` auth required |
| `session/list` | supported | `-32000` until authenticated | **`-32601`** |
| `session/resume` | supported | `-32000` until authenticated | **`-32601`** |
| `session/fork` | advertised, answers `-32603` | `-32000` until authenticated | `-32601` |

That table is the **registry's** answer and it is a year of packaging
drift away from what is on this machine. Measured live on 2026-09-10, by
speaking the protocol to each agent directly rather than through this
client, so that nothing here is an artifact of our own end:

| | Claude Code | Codex | Gemini CLI |
| --- | --- | --- | --- |
| package | `@zed-industries/claude-code-acp` | `@agentclientprotocol/codex-acp` | `@google/gemini-cli` |
| version measured | 0.16.2 | 1.11.0 | **not measured** |
| `agentInfo.title` | `Claude Code` | `Codex` | — |
| `protocolVersion` | 1 | 1 | — |
| auth methods offered | `claude-login`, described as "Run `claude /login` in the terminal" | `api-key`, `chat-gpt` | — |
| `session/new` unauthenticated | succeeds | `-32000` Authentication required | — |
| `session/list` unauthenticated | succeeds | `-32000` Authentication required | — |
| session capabilities advertised | `fork`, `list`, `resume`, `loadSession` | those four plus `close`, `delete`, `additionalDirectories`, `subagents` | — |

Two things about that table are worth more than the cells.

**The Claude row is a different package from the one above it.** The
registry names `@agentclientprotocol/claude-agent-acp` at 0.75.x; what is
installed here and what every measurement in this document now refers to
is `@zed-industries/claude-code-acp` at 0.16.2. They are not the same
artifact on a later version number, and a claim carried over from one to
the other has not been measured.

**`session/resume` and `session/fork` are not re-measured, deliberately.**
The obvious probe — open a session, then ask to resume it — answers
`-32603` from both agents, and both say why in `data.details`: Codex
reports "no rollout found for thread id …". That is a session which was
never persisted, so the probe is measuring a condition of its own making
and cannot tell "this agent does not implement resume" from "there was
nothing to resume". A probe that would settle it prompts the session,
lets it persist, and resumes it from a *second* process. Until that is
run, the registry's rows stand and this document says only that ours did
not test what it looked like it was testing.

Gemini CLI is still unmeasured and #729 is still the reason: this machine
has a `~/.gemini` with no credential in it. That is worth stating plainly
rather than leaving as an old row, because Gemini is the one agent whose
behaviour the client-side thesis most wants to know.

Four things a client has to be built around, none of which is visible
from the specification alone:

1. **Authentication is the first thing that happens, not the last.**
   Two of the three refuse `session/new` with `-32000` until
   authenticated. A client that treats `initialize` succeeding as "ready
   to work" fails on two of three agents.
2. **The auth *kind* differs, and one of them needs us to be a
   terminal.** Agent Auth means the agent opens a browser and runs its
   own OAuth callback server; the client's part is to trigger it and
   wait. Terminal Auth means the client **relaunches the agent binary
   with extra arguments in an interactive terminal** — which is what
   Claude Agent asks for. For a shell that is an unusually good fit and
   for an editor it is a whole subsystem: it is the one place where
   being a shell makes us a *better* ACP client than the tools this
   protocol was written for.
3. **An advertised capability is not a working method.** Claude Agent
   advertises `sessionFork` and answers `-32603` to it. A client must
   degrade on the *answer*, not only on the advertisement.
4. **`-32601` is a fact, not a failure.** Gemini answers it for every
   optional method. A client must treat method-not-found as "this agent
   does not do that" and carry on.
5. **An agent may decline a capability it was offered**, and whether it
   does is per agent and per *kind* of action rather than per agent
   alone. Re-measured 2026-09-10 with `terminal: true` and both file
   methods advertised, asked in as many words to run a shell command:
   **Claude Code 0.16.2 called `terminal/create`; Codex 1.11.0 ran it in
   its own process** and called no client method at all. For *files*
   both declined: neither called `fs/read_text_file` or
   `fs/write_text_file` in any measured turn, and under `-deny
   write:/**` both wrote the file themselves and it appeared.

   The earlier text here said both agents declined for commands too. It
   was measured through #1777, with this client answering permission
   requests in ids the agents had never offered, so an agent that was
   being *refused* was written down as an agent that had *declined*. See
   the client-side section for the re-measurement.

   This is the sharpest limit on the whole client-side thesis and it is
   not a defect in the implementation: the gate can only see what an
   agent *asks* for, and an adapter that shells out for itself asks for
   nothing. `terminal/*` is what makes the honest route exist, and
   whether an agent takes it is the agent's. Gemini CLI is native ACP
   rather than an adapter and is the one most likely to; measuring that
   needs a credential and is **#729**.

   The consequence for a person is worth stating plainly, and the
   re-measurement turned it around: against the two adapters measured
   today, `-deny` and the audit trail cover **the commands Claude Code
   asks us to run** — and cover neither agent's **files**, because
   neither has ever asked us to open one. The old wording said the
   opposite of both halves. It was written from the confounded
   measurement, and a sentence about what a policy covers is exactly the
   sentence that must not be inherited from a run where every permission
   answer was being discarded.

6. **A permission option's id belongs to the agent that offered it, and
   an id it did not offer is not an answer.** Measured 2026-09-10
   against `@zed-industries/claude-code-acp` 0.16.2, which offers
   `allow_always`, `allow` and `reject` where this shell's own agent
   side offers `allow-once`, `allow-always`, `reject-once` and
   `reject-always`. A client that answers with a constant of its own is
   not merely non-conforming in the abstract: that adapter read the
   unknown `allow-once` as no grant, reported the tool call to the model
   as rejected, and ended the turn having run nothing — an
   allow-everything flag that silently allowed nothing. Select by
   **kind** out of the options the request carried, and answer
   `cancelled` when the agent offered no option of that kind, because
   there is then no id to send. That is #1777, and it is why the
   keystroke mapping was already keyed on kind: the same reasoning had
   been applied to the path a person types on and not to the two paths
   that answer without one.

The differences are recorded here rather than discovered per agent
because they are the compatibility surface, and because "they all speak
ACP" is exactly the assumption that makes a client work with one of
them.

## Interactive: ACP is non-interactive, and the REPL is untouched

On the **agent** side an ACP session is non-interactive in the same
sense a script is: no line editor, no history, no prompts, no
`/dev/tty`, no terminal handoff, and `i` is not in `$-`.

Three reasons, in order of how load-bearing they are. **The channel is
taken** — the REPL reads standard input and writes standard output, and
ACP *is* standard input and standard output. **There is no terminal** —
the editor spawns us with pipes, and a prompt drawn into a pipe is not a
prompt. **The protocol has its own answer** — `elicitation/create` is
the message for a script that genuinely needs a person, and a client
that advertises the capability could serve a `read` properly. That is a
real feature and it is out of scope here.

The **client** side is where a person is present, and it does not change
that answer either: the shell that is running an agent may well be a
prompt, but the agent's own stdio is a pipe we own, and the permission
questions it asks are drawn by whatever front end the person is using.
The two front ends share `driver.Shell`, the runner it builds, the gate
and the sink. An ACP session and a prompt session are the same shell.

## Package layout

    internal/jsonrpc/      JSON-RPC 2.0 over a newline-delimited stream
    internal/acp/          the shared core — neither side's
      wire.go              the v1 message shapes, for both parties
      permission.go        option kinds ↔ interp.Decision, and the memory
      session.go           id, cwd, one turn at a time, cancellation
    internal/acp/agent/    role: we answer
    internal/acp/client/   role: we ask, and we provide the world
    cmd/sh                 `sh --acp` serves; `sh --acp-connect` calls out

The framing began as `internal/acp/jsonrpc.go` and moved out when a
second protocol needed it — see `docs/design/plugins.md`, which chose
this transport over gRPC. What moved is the framing and nothing else: the
message shapes, the method names and the reserved `-32000` are ACP's and
stayed here.

`internal/` per the promotion rule; the consumer that earns promotion is
the binary. Putting both behind `cmd/sh` gets `-dialect` for free, keeps
one place that knows how to build a `driver.Shell` from a dialect
package, and makes the agent command line an editor writes something a
person can also type.

`driver` gains what a long-lived front end needs and no more: `Session`,
a shell held open across several inputs, and `Shell.Dir`, because
`session/new` carries a per-session working directory and a process has
only one.

## What this is not

It is not a sandbox, and the boundary is around the interpreter rather
than around the process tree. Approving `cat /secret` approves
*starting* `cat`; what `cat` then reads is its own business. On the
client side the same limit applies to the agent's own process: what it
does through `fs/*` and `terminal/*` is ours to gate, and what it does
with its own file descriptors is not. Containment is
`docs/design/sandboxing.md`.

## Decisions flagged for review

Each is a defensible default chosen so the work could proceed; each is
genuinely the maintainer's.

1. **Protocol version 1 rather than the v2 alpha.** Revisit when v2
   stabilizes. All three agents speak 1 today.
2. **The escalation set** — exec, writing opens, signals; reads recorded
   and not asked about. The decision most likely to be wrong.
3. **`allow_always` scope**: exact (kind, path), session-lifetime.
4. **stderr as `agent_message_chunk` with a `_meta` marker** rather than
   a distinct update kind, which v1 does not have.
5. **`sh --acp` and `sh --acp-connect` rather than separate binaries.**
6. **`session/load` refused on the agent side.** A shell's session state
   is not in the transcript.
7. **No permission timeout.** Cancellation is the way out.
8. **`fs/*` and `terminal/*` not *called* on the agent side**, and
   **implemented and advertised on the client side.** The asymmetry is
   deliberate and is the same rule read twice: keep accesses inside our
   boundary.
9. **Terminal Auth is in scope for the client.** Without it Claude Agent
   cannot be authenticated from this shell at all, because `terminal` is
   the only method it offers.
10. **No default authentication method.** `-acp-auth` names one or nothing is
    attempted, rather than the client choosing when an agent offers exactly
    one. Picking a credential path for somebody is a guess about their
    account, and a guess that succeeds is worse than one that fails.

## Staging

1. the design (this document);
2. the shared core — framing and wire types, then the permission
   mapping and the session model;
3. `driver.Session` and `Shell.Dir`, which both roles need;
4. the agent side, and `sh --acp`;
5. the client side, and `sh --acp-connect`, against all three agents.

`Closes #493` belongs on the last of those and on nothing before it.

### The fifth stage, as amended

That last line said "against all three agents", and read literally it meant
three completed round trips. Two of them complete; the third needs a Gemini
credential, which is a human action nobody working on this repository can
perform on somebody else's account — so the initiative was blocked on
something that is not code and would have held the first tag with it.

**Decided by the maintainer, 2026-09-06: Gemini's credential is #729 at P2 and
does not block v0.0.0.** The criterion becomes:

> All three agents are **driven**, and any that cannot complete a session
> **reports exactly why, with its advertised methods**.

That is a bar and not a formality, and it is met by measurement rather than by
assertion. Gemini CLI 0.58.0 reaches it on both of its paths:

```
sh: connected to gemini-cli 0.58.0, protocol 1
sh: gemini-cli 0.58.0 needs authenticating first: jsonrpc -32000: Gemini API key is missing or not configured.
sh:   oauth-personal [agent] (Log in with Google): Log in with your Google account
sh:   gemini-api-key [agent] (Gemini API key): Use an API key with Gemini Developer API
sh:   vertex-ai [agent] (Vertex AI): Use an API key with Vertex AI GenAI API
sh:   gateway [agent] (AI API Gateway): Use a custom AI API Gateway
sh: choose one with -acp-auth oauth-personal
```

and, with a method named and **accepted**, which is the path that had to be
fixed to meet the bar:

```
sh: gemini-cli 0.58.0 accepted -acp-auth gemini-api-key and still refuses a session: jsonrpc -32000: Gemini API key is missing or not configured.
sh: the method was settled, so what is missing is the credential behind it
sh: rather than the choice of method — supply it outside this shell.
sh:   gemini-api-key [agent] (Gemini API key): Use an API key with Gemini Developer API   <- the one -acp-auth named
```

**Why that second one was work rather than wording.** Gemini answers
`authenticate` with `{}` and then refuses `session/new` anyway, so the old
message told a person who *had* authenticated that they needed to authenticate
first, and then suggested the first advertised id — which, after they had used
one, is the method they did not choose being offered as though nothing had
happened. A person following that goes round the same flag again. What is
missing is the credential behind an accepted method, and saying so is the
difference between somebody going to get an API key and somebody retrying a
flag. `TestASessionRefusedForAuthenticationSaysExactlyWhy` holds it.

An agent that hangs, that fails generically, or that says nothing does **not**
meet this criterion, and neither does one this shell cannot drive at all.
