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

### Where the gate sits on this side

Every inbound request that would touch the world is an `interp.Action`
before it is anything else:

| inbound | action asked of the gate |
| --- | --- |
| `fs/read_text_file` | `ActionOpen`, `Write: false` |
| `fs/write_text_file` | `ActionOpen`, `Write: true` |
| `terminal/create` | `ActionExec` with the argv |
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

The ACP gate wraps an inner `interp.Gate` — nil today, a sandbox policy
tomorrow:

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

### The correlation gap: blocked on #719, and not ours to close

`interp.Event` carries no identity for the action it belongs to.
`EventCommandStart` and `EventCommandEnd` are matched by *ordering*, and
ordering is exactly what concurrency breaks: a background job and each
half of a pipeline emit from their own goroutines.

The audit schema in `internal/event` — owned by
`docs/design/sandboxing.md`, which this consumes rather than competes
with — does not close it either, and says so: `seq` is a total order of
*emission* and is explicitly "not a causal order". That is the right
call for a log and leaves this mapping without an answer.

**Two consumers reached the same missing field from opposite
directions.** The blocks work needed to say which events belong to which
run and had to generate an id of its own to cope; this needs to say
which events belong to which action, and to join a permission request to
the events for the action it approved. It is filed as **#719**, and the
reason it is one issue rather than two workarounds is that *two id
schemes that do not agree are worse than none* — they look joinable and
are not.

So this front end does **not** invent one. Until #719 lands it matches
by a fingerprint of the action alone — kind, path, args, and the write
flag or the signal target — with a queue of open tool calls per
fingerprint, and an unmatched end becomes a standalone completed tool
call rather than being dropped. The line and the file would discriminate
better and are deliberately left out: a gate is consulted with an
`Action` before any event exists, so a key carrying them could never
join the two halves. Where two identical commands are in flight at once
it marks the wrong tool call complete, which is cosmetic — no decision
changes — and is still wrong.

The tool call ids and session ids this front end does mint are not that
scheme and are not a substitute for it. `ToolCallId` and `SessionId` are
*required by the protocol*: a client cannot show a permission request
without one, and every message in a session names it. They identify
things on the wire, not actions in the interpreter, and when #719 lands
the fingerprint goes and these stay.

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

    internal/acp/          the shared core — neither side's
      jsonrpc.go           JSON-RPC 2.0 over a newline-delimited stream
      wire.go              the v1 message shapes, for both parties
      permission.go        option kinds ↔ interp.Decision, and the memory
      session.go           id, cwd, one turn at a time, cancellation
    internal/acp/agent/    role: we answer
    internal/acp/client/   role: we ask, and we provide the world
    cmd/sh                 `sh --acp` serves; `sh --acp-connect` calls out

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

## Staging

1. the design (this document);
2. the shared core — framing and wire types, then the permission
   mapping and the session model;
3. `driver.Session` and `Shell.Dir`, which both roles need;
4. the agent side, and `sh --acp`;
5. the client side, and `sh --acp-connect`, against all three agents.

`Closes #493` belongs on the last of those and on nothing before it.
