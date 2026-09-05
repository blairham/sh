# ACP: an Agent Client Protocol front end

The gate and the event stream are the shell's native permission-and-audit
surface, and they were built for three consumers at once: a sandbox, an
AI assistant, and an agent protocol. This is the design for the third.
It is a **fourth consumer of `driver.Shell`**, beside `-c`, a script
file and the prompt, and it adds no new question to the interpreter: it
answers the two that are already there.

Read `docs/design.md` first. The claim this document depends on is the
one made there — that a policy enforced outside the interpreter does not
reach inside it — because it is what makes an agent front end honest.
An editor that spawns `sh -c` and watches the pipe sees what a script
chose to print. An ACP front end over this seam sees every exec, every
open, every stat and every signal, including the ones inside an `eval`.

## Which revision, which transport, and how that was established

**Protocol version 1**, as published by the Agent Client Protocol
project, wire schema `schema/v1` at release 1.21.0 (2026-08-20).
`protocolVersion` is the integer `1`; it is bumped only for breaking
changes, and everything else is negotiated through capabilities.

**stdio**: the client launches the agent as a subprocess, and JSON-RPC
2.0 messages cross on the agent's standard input and standard output,
UTF-8, one message per line, with no embedded newlines. The agent may
write logs to standard error and **must not** write anything to standard
output that is not an ACP message. That last sentence has a consequence
for a shell and it is not a small one — see "The three streams" below.

Every message shape in this document was taken from the machine-readable
schema rather than from prose: `schema/v1/schema.json` (170 definitions,
each carrying `x-side` and `x-method`) and `schema/v1/meta.json`, which
is the method table. The narrative pages —
`docs/protocol/v1/{transports,initialization,prompt-turn,tool-calls}` —
were read for the rules that a JSON schema cannot express, such as which
party must answer a pending permission request when a turn is cancelled.
Nothing here was inferred from an SDK's source.

A version 2 exists and is **not** targeted. It is `2.0.0-alpha.3`, it
drops `session/load`, `authenticate` and the whole `fs/*` and `terminal/*`
client surface in favour of a reshaped one, and shipping a shell against
an alpha would mean re-cutting the mapping when it moves. Version 1 is
what clients speak today. The wire types live in one file for exactly
this reason: when v2 stabilizes, the mapping below is unchanged and only
the encoding moves.

## Which side of the protocol a shell is

The shell is the **Agent**. The editor is the Client.

This falls out of the direction the two interesting messages travel.
`session/request_permission` goes agent → client, which is a gate
consultation asking a person. `session/update` goes agent → client,
which is the event stream. `session/prompt` goes client → agent and
carries the thing to run. A shell that were the *client* would be
hosting somebody else's agent, which is a different program and not
this issue.

So: an editor spawns `sh -acp`, sends it a command, watches what the
shell does, and answers when the shell asks whether it may.

## The subset a shell meaningfully serves

ACP is written for agents that talk to a model. A shell has no model, no
plan and no token budget, and the honest subset is smaller than the
protocol. What is implemented:

| method | direction | what it means here |
| --- | --- | --- |
| `initialize` | client → agent | version and capability negotiation |
| `session/new` | client → agent | a shell, with the client's `cwd` |
| `session/prompt` | client → agent | run this text as shell |
| `session/cancel` | client → agent (note) | interrupt the running turn |
| `session/update` | agent → client (note) | what the shell just did |
| `session/request_permission` | agent → client | the gate, asking a person |

What is deliberately refused with JSON-RPC `-32601`, and why:

- **`authenticate` / `logout`.** `authMethods` is empty. A local shell
  authenticates by being a process the user already started; inventing
  a login for it would be theatre.
- **`session/load`, `session/resume`, `session/list`.** `loadSession`
  is advertised false. Replaying a shell session is not replaying a
  transcript — the state that matters is variables, functions, the
  working directory and the descriptor table, none of which is in the
  update stream. A load that restored the *text* and not the shell
  would be a lie about what the client is looking at.
- **`session/set_mode`, `session/set_config_option`.** There are no
  modes and no options yet. The obvious candidate is the dialect, and
  it is not free: a dialect is runtime state that `set -o posix`
  re-reads mid-script, so exposing it as a session config option needs
  a `current_mode_update` every time a script changes it. Worth doing,
  worth doing on purpose.
- **MCP servers.** `session/new` carries `mcpServers`; ours is accepted
  and ignored, with `mcpCapabilities` advertised false. A shell is not
  an MCP host.

What is not used from the *client* side: `fs/read_text_file` and
`fs/write_text_file` are not called even where the client advertises
them, and this is the sharpest decision in the document. A shell reads
files by opening them, through the gate, in the interpreter. Routing a
`cat` through the client's file system capability would move that read
*outside* the boundary this whole seam exists to draw, and the audit
trail would lose it. The same holds for `terminal/*`: the shell has its
own process model, decided in `docs/design.md`, and running a command
through the client's terminal would put the process outside the gate.
A client that wants those accesses represented sees them as tool calls
and permission requests, which is a truer picture than borrowing the
client's hands to do the work.

## What a prompt is, when the agent is a shell

`session/prompt` carries an array of content blocks. The text blocks,
joined with newlines, are the shell program for the turn. It is run by
the `-c` route on the session's runner, which is `driver.RunCommand`'s
shape: the origin is labelled `-c` in a parse failure's location, `$0`
is the shell, and there are no positional parameters.

`promptCapabilities` are all advertised false, which is the baseline —
text and `resource_link` — and non-text blocks are accepted and skipped
rather than refused, because the baseline requires an agent to tolerate
a resource link and there is nothing useful a shell does with one. A
prompt with no text at all is an empty program and ends the turn with
`end_turn` and nothing run, which is what an empty `-c` string does.

The response's `stopReason` is `end_turn` when the program finished,
whatever its exit status — a failing command is a turn that completed,
not a turn that stopped — and `cancelled` when the turn was interrupted.
`refusal` is reserved for a program that could not be parsed at all;
`max_tokens` and `max_turn_requests` have no meaning here and are never
sent.

**One turn at a time per session.** A second `session/prompt` for a
session with a turn in flight is refused with `-32602`, because the
runner is a single shell and interleaving two programs in it would give
neither one the variables it wrote.

## The three streams

An ACP agent may not write to standard output anything that is not a
protocol message, and a shell's whole job is writing to standard output.
The resolution is that the shell's stdout and stderr are **not the
process's**. `driver.Shell` already takes them as `io.Writer` fields, so
the session hands the runner writers of its own, and each write becomes
a `session/update`:

- stdout → `agent_message_chunk`, a text content block;
- stderr → `agent_message_chunk` as well, on the same turn.

Keeping stderr distinguishable is a real want and there is no honest
place for it in v1's update vocabulary — `agent_thought_chunk` means
model reasoning and would be a misuse. It is carried in `_meta` on the
chunk, which is what `_meta` is for, and flagged below for review.

Standard *input* is empty. A `read` in an ACP session gets end of file.
That is the deliberate answer to the interactive question below, not an
oversight, and ACP has a proper home for it — `elicitation/create` — the
day we want it.

Bytes are not copied into the event stream, and `interp.Event` was
designed not to carry them: the consumer that wants output taps the
writer it supplied, which is exactly what this front end does.

## The gate becomes a permission request

This is the heart of it.

`interp.Gate` answers `Allow` or `Deny`, synchronously, from whatever
goroutine reached the action. The ACP gate answers by asking a person
over the wire, and blocks that goroutine until the client replies. A
background job asking while the foreground asks is two concurrent
requests, which JSON-RPC handles by construction — each carries its own
id — so the gate keeps no lock across the round trip beyond the one that
protects its memory of past answers.

### Which actions reach a person

Not all of them, and this is a judgement rather than a reading of the
protocol. A single `ls | grep x` stats every PATH entry it tries; a glob
stats and reads directories in bulk. A permission prompt arriving at
that rate is a prompt people click through, which is a *worse* boundary
than an honest record, because it converts a considered answer into a
reflex.

So the escalation set defaults to the actions that change something
outside the shell or start something the boundary can no longer see:

- **`ActionExec`** — always. A command is the thing a person means to
  approve, and once it is running its own accesses are its own.
- **`ActionOpen` with `Write`** — always. This is the shell modifying
  the file system in its own right.
- **`ActionSignal`** — always. Reaching another process is the same
  shape of act as starting one.

Reads — a non-writing open, a stat, a directory read — are allowed and
**recorded**, never asked about. `ActionInherit` is not gated at all, by
the interpreter's own contract.

The set is a value on the front end and not a constant, so a caller that
wants everything escalated can have it. What it is *not* is a policy
language: refusing reads by rule is the sandboxing work, and this front
end composes with it rather than duplicating it.

### Composition with a real policy

The ACP gate wraps an inner `interp.Gate`, which is nil today and is a
sandbox policy tomorrow. The order is:

1. the inner gate is consulted first; if it denies, the action is denied
   and **no one is asked**. A policy refusal is not negotiable, and
   offering a person a button that overrides the sandbox would make the
   sandbox advisory;
2. otherwise, if the action is in the escalation set and no remembered
   answer covers it, the person is asked;
3. otherwise it proceeds.

This is why the front end does not need a third `Ask` decision value in
`interp`. The interpreter's question stays binary; *who* answers it is
the front end's arrangement.

### The four option kinds

Every escalated action offers all four of ACP's `PermissionOptionKind`
values, with stable option ids:

| optionId | name | kind | effect |
| --- | --- | --- | --- |
| `allow-once` | Allow | `allow_once` | `interp.Allow`, remembered nowhere |
| `allow-always` | Allow always | `allow_always` | `interp.Allow`, remembered |
| `reject-once` | Reject | `reject_once` | `interp.Deny` |
| `reject-always` | Reject always | `reject_always` | `interp.Deny`, remembered |

An unrecognized `optionId` in the response is a **deny**, not a guess.

**What "always" remembers** is the pair (action kind, path) — for an
exec that is the resolved program, for an open the file and whether it
was for writing, and for a signal the target pid, which is deliberately
narrow because a pid is not a stable identity and a remembered answer
about one is nearly useless on purpose. The memory lives on the
**session**, not the connection: sessions have their own working
directory and a client presents them as separate things, so an answer
given in one should not silently govern another.

It is an exact match and not a prefix. "Allow always for everything
under /tmp" is a policy language, and writing one here would be the
sandboxing initiative done badly in the wrong package.

### Deny until answered

Every path that is not an explicit allow is a denial:

- outcome `cancelled` (which the client **must** send for pending
  permission requests when the turn is cancelled) → `Deny`;
- an `optionId` naming a reject kind, or naming nothing we offered →
  `Deny`;
- a JSON-RPC error from the client → `Deny`;
- the connection closing with the request outstanding → `Deny`;
- the turn's context being cancelled while waiting → `Deny`.

There is no timeout. A client that never answers holds the goroutine,
and that is correct: the alternative is a deadline that silently allows
or silently refuses, and either one is a boundary that reports something
other than what happened. Cancellation is the intended way out, and it
is a message the protocol already has.

A denial is reported by the interpreter in its own words — a denied exec
says so and fails with 126, a denied stat is quiet and reads as a
missing path — and the front end does not second-guess that. The
`EventDenied` that follows becomes a failed tool call, so the client
sees the refusal it caused.

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
`rawInput` carries the structured action — kind, path, args, write flag,
pid, signal — which is the audit record the protocol will let us hand
over verbatim.

An escalated action creates its tool call *before* the permission
request, because `session/request_permission` requires a `toolCall` and
the client will want to show what it is asking about. The gate runs
before `EventCommandStart`, so the tool call exists by the time the
event arrives and the event updates it rather than creating a second.

### The correlation gap, which is not ours to close

`interp.Event` carries no identity for the action it belongs to.
`EventCommandStart` and `EventCommandEnd` are matched by *ordering*, and
ordering is exactly what concurrency breaks: a background job and each
half of a pipeline emit from their own goroutines, so two runs of the
same command can interleave their start and end.

Until the event schema carries an id, this front end matches by a
fingerprint of (kind, path, args, line, file) with a FIFO of open tool
calls per fingerprint, and an unmatched end becomes a standalone
completed tool call rather than being dropped. It is cosmetic when it is
wrong — the *wrong tool call* is marked complete, no decision changes —
but it is wrong, and the fix is one field.

**The stable event schema is owned by the native sandboxing work
(#494),** because the sandbox audit log and these session updates are
the two consumers of it. What is wanted from it is a monotonic
per-action id on `Event` (and the same id on the `Action` a gate is
consulted about, so that a permission request and the events for the
action it approved are provably the same action). This front end does
not define one.

## Interactive: ACP is strictly non-interactive, and the REPL is untouched

The REPL and the ACP front end do **not** share the interactive
plumbing. An ACP session is non-interactive in the same sense a script
is: no line editor, no history, no prompts, no `/dev/tty`, no terminal
handoff, and `i` is not in `$-`.

Three reasons, in order of how load-bearing they are.

**The channel is taken.** The REPL reads standard input and writes
standard output; ACP *is* standard input and standard output. There is
no arrangement in which both are on the same descriptors and both work.

**There is no terminal.** The REPL's editor needs raw mode, a window
size and `/dev/tty` for job control. A client spawns an agent with
pipes. A prompt drawn into a pipe is not a prompt.

**The protocol has its own answer.** Where a script genuinely needs a
person — a `read` at a prompt, a password — ACP's `elicitation/create`
is the message for it, and a client that advertises the capability could
serve a `read` properly. That is a real feature and it is out of scope
here; noting it is the point of writing this section.

What the two front ends *do* share is everything that matters:
`driver.Shell`, the runner it builds, the gate and the sink. An ACP
session and a prompt session are the same shell.

## Package layout

    internal/acp/        the protocol and the mapping
      jsonrpc.go         JSON-RPC 2.0 over a newline-delimited stream
      wire.go            the v1 message shapes we use, and nothing else
      agent.go           initialize / session lifecycle / dispatch
      session.go         one shell: runner, turn, streams
      gate.go            Gate → session/request_permission
      updates.go         Sink and stream writers → session/update
    cmd/sh               `sh -acp` serves ACP on stdin/stdout

`internal/` per the promotion rule: a package is promoted once something
has consumed it, and the consumer that earns it is the binary. Putting
the ACP front end behind `sh -acp` rather than in a binary of its own
gets `-dialect` for free, keeps one place that knows how to build a
`driver.Shell` from a dialect package, and makes the agent command line
an editor writes something a person can also type. If a client ever
needs `acp` as a separate executable, `cmd/acp` is a `main` that calls
one function.

`driver.Shell` gains one field for this: **`Dir`**, the directory the
shell starts in, defaulting to the process's when empty. `session/new`
carries a `cwd` per session and a process has one working directory, so
without it two sessions would have to fight over `os.Chdir` — which is
the library-purity rule from `docs/design.md` reappearing one level up.
It is a field every other consumer already had implicitly.

## What this is not

It is not a sandbox, and the boundary is around the interpreter rather
than around the process tree. Approving `cat /secret` in an editor's
permission dialog approves *starting* `cat`; what `cat` then reads is
its own business and the gate never sees it. This front end makes the
shell's own accesses visible and refusable, which is a strictly larger
surface than an editor spawning `sh -c` has today, and strictly smaller
than containment. Containment is #494.

## Decisions flagged for review

Each of these is a defensible default chosen so the work could proceed;
each is genuinely the maintainer's.

1. **Protocol version 1 rather than the v2 alpha.** Revisit when v2
   stabilizes.
2. **The escalation set** — exec, writing opens, signals. Reads
   recorded and not asked about. This is the one that decides whether
   the front end is usable, and the one most likely to be wrong.
3. **`allow_always` scope**: exact (kind, path), session-lifetime. A
   subtree or a pattern would need a policy language.
4. **stderr as `agent_message_chunk` with a `_meta` marker** rather
   than a distinct update kind, which v1 does not have.
5. **`sh -acp` rather than `cmd/acp`.** A flag on the substrate's
   driver, not a new binary.
6. **`session/load` refused.** A shell's session state is not in the
   transcript.
7. **No permission timeout.** Cancellation is the way out.
8. **`fs/*` and `terminal/*` client capabilities not used**, even when
   advertised, because using them would move accesses outside the gate.

## Staging

1. this document;
2. `internal/acp`: JSON-RPC framing and the wire types, tested against
   the schema's shapes;
3. the session: the `driver.Shell` consumer, the gate mapping, the
   update mapping;
4. `sh -acp` and an end-to-end test that drives a real connection.
