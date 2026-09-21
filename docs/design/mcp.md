# MCP: a Model Context Protocol server over the same seam

`docs/design/acp.md` opens by saying the gate and the event stream "were
built for three consumers at once: a sandbox, an AI assistant, and an
agent protocol." This is the **fourth** consumer of that seam, and the
whole design fits in one sentence: an MCP client asks the shell to run a
command, and the command goes through the gate a script's command goes
through.

Read `docs/design.md` and `docs/design/acp.md` first. Everything below
assumes both.

## What is already decided, and stays decided

**This shell is not an MCP host.** `docs/design/acp.md:136` says
`session/new` carries `mcpServers`, that ours is accepted and ignored,
and that a shell is not an MCP host. That line is about *consuming* MCP
servers and it is unchanged. Being one is the other direction.

## Why a second protocol at all

Two reasons, and neither is "because MCP exists".

**Reach.** MCP is spoken by most coding agents and assistants; ACP is
essentially one editor family. If the goal is "an AI runs commands
through our gate", MCP is where the clients already are.

**A command line somebody controls.** This is the sharper one. A coding
agent runs what it wants run as `$SHELL -c '<command>'`, and #1334
records the consequence: there is **nowhere to put a flag**. An MCP
server is configured with a *user-controlled command line* —

    claude mcp add shell -- /usr/local/libexec/sh/sh -mcp -dialect bash \
        -policy ~/.config/agent.policy

— so the policy rides along on the invocation, and `driver` installs it
before the protocol starts for exactly that reason.

`--policy` in `$SHELL` reaches the same place by a different route and
`docs/install.md` says how; the two are alternatives rather than
replacements, since a launcher that execs `$SHELL` verbatim takes no
arguments and an MCP configuration always does.

## The revision implemented against

**`2026-07-28`**, the current released specification, with the
**`2025-11-25`** handshake served beside it.

That revision is *modern* in its own words: there is no `initialize`
handshake, every request declares its version in
`_meta["io.modelcontextprotocol/protocolVersion"]`, `server/discover` is
the probe every server must answer, and every result carries
`resultType`.

The second era is a **measurement**, not a hedge. Measured 2026-09-21
against Claude Code 2.1.267 — the client population this work is for —
by standing a logging server up under `claude mcp add` and reading what
arrived:

    {"method":"initialize","params":{"protocolVersion":"2025-11-25",
      "capabilities":{"roots":{"listChanged":true},"elicitation":{}},
      "clientInfo":{"name":"claude-code",…}},"jsonrpc":"2.0","id":0}
    {"jsonrpc":"2.0","method":"notifications/initialized"}
    {"method":"tools/list","jsonrpc":"2.0","id":1}

No `_meta` at all, no `server/discover` probe, and it accepted a server
answering `2025-06-18` to a request for `2025-11-25`. A modern-only
server would be correct against the specification and **unreachable by
the client the issue names**. The specification provides for precisely
this: its "Backward Compatibility with Initialization-Based Versions"
describes a *dual-era* server and says a dual-era server "selects its
behavior from how the client opens".

So the era is a property of **each request** rather than of the
connection, which is also what a stateless protocol requires: a request
carrying the modern `_meta` is answered in the modern shape, and one
without it is answered in the legacy shape. `TestOnlyAModernResultCarriesResultType`
sends both down one connection.

That measurement is committed as a test, verbatim
(`TestTheHandshakeAMeasuredClientSendsIsAnswered`), because a test
written from the specification alone could not have found it.

## The mapping: a command is a handle

This is the part that does not port from ACP, and #1338 reserved it as a
decision rather than something to guess. An MCP tool call is one request
and one result; a shell command is four things at once — a stream, an
exit status, two output channels, and possibly standard input. The three
honest mappings were:

1. **One tool, one result.** `run(command) -> {stdout, stderr, status}`.
2. **A handle.** `create`, then `output`, `wait`, `kill`, `release`.
3. **Both**, with (1) as a bounded convenience over (2).

The maintainer decided on 2026-09-21: **mirror ACP's terminal model, and
streaming rides MCP progress notifications.** (1) is rejected outright,
for the reason the issue gives — it is the answer that looks fine until
somebody runs `tail -f`, where it either blocks forever or truncates,
and truncating a command's output while returning a success status is
the silent-wrong-answer class this repository treats as its worst
failure.

So the tools are ACP's five verbs:

| MCP tool | ACP method |
| --- | --- |
| `terminal_create` | `terminal/create` |
| `terminal_output` | `terminal/output` |
| `terminal_wait_for_exit` | `terminal/wait_for_exit` |
| `terminal_kill` | `terminal/kill` |
| `terminal_release` | `terminal/release` |

Underscores rather than the dots MCP's naming rules also permit: dots are
legal in this revision and were not always, and a client that rejects a
tool name it cannot parse rejects it *silently*, leaving a server that
connects and serves nothing.

**Option 3's convenience is deliberately not built.** The decision chose
the handle model and said nothing about a one-shot wrapper over it, and
the open question the issue itself flagged — what such a tool does when
it hits its bound, refuse by name or return what it has — is exactly the
sort of thing that is expensive to undo once clients depend on it.
Adding it later is additive; taking it away is not.

## Streaming

`terminal_wait_for_exit` is the streaming call, and it is the only one it
could be. The revision allows a server to send `notifications/progress`
only against a **request that is in flight**, so the stream belongs to
the call that is waiting. A client that sends a `progressToken` in that
call's `_meta` is sent what the command writes as it writes it; a client
that sends none gets the same answer with nothing in between, and
`terminal_output` still holds everything either way.

Two rules make the stream honest, and both are tested by mutation:

- **The progress value counts every byte the command has *ever*
  written**, not what is currently retained. The revision requires the
  value to increase with each notification, and a buffer with an
  `outputByteLimit` on it *shrinks*. `termhost.Terminal.Since` is what
  answers that question, and it is the one piece of the terminal
  machinery neither protocol had before.
- **Nothing is sent when nothing was written.** A tick with no new bytes
  has no honest number, so it is skipped rather than repeated.

The messages concatenate to exactly what `terminal_output` holds, which
is asserted rather than asserted-about: a client that read only the
notifications has the output.

## What is shared with ACP, and by what mechanism

This is the constraint the work was accepted under. Two protocol front
ends over one seam is this repository's duplicate-rule hazard — five
instances, most recently one where a local copy of a rule *prevented*
the general fix (#1189) — and writing the terminal verbs twice is how
they drift.

Shared **by construction**, not by discipline:

| what | where | how |
| --- | --- | --- |
| the terminal verbs: create, output, wait, kill, release | `internal/termhost` | one implementation, two front ends call it |
| the exec/line reading, the gate call, the audit record | `internal/termhost.Host` | same |
| the output buffer, its limit, its rune-safe trim | `internal/termhost.Terminal` | same |
| running a command line as this shell runs one | `internal/termhost.Interpreter` | `acpboot.Interpreter` is now an **alias** of it |
| the ACP wire types for output and exit status | `internal/acp` | **type aliases** of `termhost.Output` and `termhost.ExitStatus` |
| `acp.TerminalCommand` | `internal/acp` | a **type alias** of `termhost.Command` |
| recording an access that was allowed and then failed | `boundary.Boundary.Failed` | one method, both front ends call it |
| what a development build reports as its version | `driver.Shell.ReportedVersion` | one method; it was a copy in `acpboot` and would have become two |
| "is this served verb gated?" | `internal/gateguard` | one detector, a table per front end |

The aliases are the `repl.PromptStyle` pattern the issue names: a name in
one package that *is* the type in another, so the two cannot disagree.

Each front end keeps only its own wire shapes and its own dispatch. That
is the line: a message shape belongs to the protocol that defines it, and
everything below the dispatch belongs to neither.

### The guard that keeps it true

`internal/gateguard` reads the source and answers, per served verb,
whether it reaches a `Boundary` method that can say *no*. Both front ends
keep a table — `reach` in `internal/acp/gateguard_test.go` and in
`internal/mcp/gateguard_test.go` — and adding a case to either dispatch
fails until the verb is declared and the declaration is true.

Two things about it changed for this work, and both were found by
mutation rather than by reading:

- It now **follows the call into the shared machinery**. Before, it
  looked only inside the handler, on the reasoning that delegation reads
  as "reaches nothing", which is a false negative in the safe direction.
  That stopped being safe the moment every terminal verb delegated: the
  guard would have failed on a correct tree, and the fix somebody reached
  for would have been to declare the verbs `inside`.
- It now requires a call that can **refuse**. Counting any `Boundary`
  call at all left the guard green when the gate was deleted from
  `termhost.Host.Create`, because that same function also reports a
  failed start through `Boundary.Failed` — a record is not a refusal.

## The permission path, and what is deliberately absent

**There is no per-action permission prompt over MCP.** The policy is what
refuses.

This is a decision and it is worth stating, because the issue's own body
says elicitation makes per-action permission expressible. Two things
argue against building it:

- **The revision forbids the shape it would need.** On stdio,
  `2026-07-28` says the server **MUST NOT** write JSON-RPC requests to
  standard output. A server that needs something from a person answers a
  tool call with an `InputRequiredResult` and is *retried* with the
  answer — which means a gate consultation reached halfway through a
  running command would have to suspend the command, unwind to the tool
  result, and be resumed on a later call. That is a large machine, and it
  is not what the decision asked for.
- **The host already asks.** The revision requires a human in the loop
  with the ability to deny a tool invocation, and the measured client
  does exactly that before every call.

So the gate the server installs is the shell's own — `sh.Gate`, straight
from the invocation — and `--policy` is the whole of the answer. That is
also the shape #1334 asked for: a policy that reaches a coding agent.
`acp.Gate`, `acp.Memory` and `acp.Decide` are the *escalation* machinery
for a front end that has somebody to ask, and this one does not; nothing
of them is copied here.

If an interactive permission path is ever wanted over MCP, the thing to
reuse is `acp.Gate` — inner gate first, its refusal final and unaskable —
with an `Asker` that speaks elicitation. It is a func field for exactly
that reason.

## Cancellation

`notifications/cancelled` is read and dropped, and that is the honest
state rather than a stub. `internal/jsonrpc` hands a handler the
*connection's* context rather than one per request, so there is nothing a
cancellation could reach — and a client that wants a command stopped has
`terminal_kill`, which is a better answer anyway: it stops the command
and keeps its output.

A blocked `terminal_wait_for_exit` therefore ends when the command ends
or when the connection does. That is the same property ACP's
`terminal/wait_for_exit` has, and it is deliberate: a wait that gave up
and reported an exit that had not happened would tell the client
something untrue about a process that is still running.

## What is not served

Tools, and nothing else. No resources, no prompts, no completions, no
logging, no subscriptions, no extensions. Each is a capability this shell
would have to have something to put in, and it does not — what is absent
is absent because it is empty, not because it is unfinished. A method
this server does not claim is refused by name rather than answered
emptily, which is the rule the ACP side follows for the same reason: a
server that answered a capability it never advertised would be telling
the client something untrue about what it can rely on.

There is also **no session**. The revision has no protocol-level session,
and its own guidance for state across calls is an explicit handle
returned by a creation tool — which is what a terminal id is. Each
command line runs on a shell built from the invocation's template, so
variables and the working directory do not carry from one tool call to
the next, exactly as they do not carry between two `sh -c` runs. A client
that wants them to say so in one line.

## How it is spelled, and why

| binary | serve |
| --- | --- |
| `sh` | `-mcp` or `--mcp` |
| `bash`, `zsh`, `ksh`, `dash`, `ash` | `--mcp` |

The dialect binaries take the **long form only**, and that is measured
rather than stylistic. Measured 2026-09-21 on macOS 25.6:

    bash 5.3.20   bash -mcp 'echo RAN'    printed RAN
    zsh 5.9.2     zsh -mcp 'echo RAN'     printed RAN

Real bash reads `-mcp` as the bundle `-m -c -p` — monitor, command
string, privileged — so a one-dash word there would shadow working
behavior rather than add a flag. The long form shadows nothing; no shell
in the panel ran the command:

    bash 5.3.20   --mcp: invalid option     status 2
    bash 3.2.57   --mcp: invalid option     status 2
    zsh 5.9.2     no such option: mcp       status 1
    ksh93u+ 2012  mcp: bad option(s)        status 2
    dash          Illegal option --         status 2

`sh` keeps both spellings, as it keeps `-policy` beside `--policy`: it is
not a shell anyone's shebang names, so it has no bundle to collide with,
and `sh -mcp` is what a person types into an agent's configuration.

`internal/mcp` imports `driver`, so `driver` cannot import it back. The
front end therefore *reads* the option and calls a hook —
`driver.Shell.ServeMCP`, nil in a library and filled in by a binary, the
same shape as `ServeACP`. A binary that supplies no hook refuses the
option rather than accepting the word and doing nothing.

The option is recorded during the option loop and acted on **after** the
boundary is installed. That ordering is the whole point: once the
protocol starts there is no invocation left to read, because a tool call
carries its own command and its own directory.

## What it does not contain

The same paragraph `docs/install.md` ends on, and it holds here
identically: **the boundary is around the shell, not around the process
tree.** A policy decides what the shell opens, stats, runs and signals. A
command the shell was allowed to start then makes its own accesses, and
nothing here sees them.

What this route *does* buy over an argv-level gate is worth naming,
because it is the reason a command with no `args` is interpreted rather
than exec'd: every exec, open and stat **inside** a command line crosses
the boundary. `-deny exec:/bin/rm` refuses the `rm` inside a pipeline
that an argv-level gate would have seen only as `/bin/sh`.
