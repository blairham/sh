# Shell injection, and where the decision point belongs

> **Status: written, not published.** This page is in the repository and has
> gone nowhere else. There is no post, no announcement and no submission
> anywhere, and publishing it is a separate step that has not been taken —
> the maintainer's call, made after reading this, not implied by its being
> merged. #1305 is where that decision is recorded.
>
> It is written as an argument rather than as a design note because that is
> what would eventually be published. Nothing here may run ahead of what the
> tree can be shown to do; every claim below was measured on the date beside
> it, and the scope limit is not an appendix.

## The class of problem

A coding agent is given a command and has to decide whether to run it. The
decision is nearly always made by looking at the command — a deny-list, a
regular expression, a model asked whether this looks dangerous — and then the
command is handed to a shell.

**GuardFall** (Cloud Security Alliance / Adversa AI, 30 June 2026, as recorded
in #1305) put a set of decades-old shell spellings through the guardrails of
eleven popular open-source coding and computer-use agents. Ten were bypassed.
The bypasses were nothing exotic: quote removal, `$IFS`, command substitution,
base64 piped into an interpreter, a destructive flag in `argv`. Their
diagnosis, quoted:

> the filter checks what the command **looks like**, bash runs what the command
> **means**, and those two things are not the same

> Because the gap is architectural, patching any single tool's deny-list does
> not resolve it; **the underlying evaluation model must change.**

That is a statement about *where* the decision is made, not about how well it
is made. A filter in front of a shell is reading text; the shell is going to
resolve, expand, substitute and split that text into an action, and only then
does an action exist to have an opinion about.

## The argument, and its limit, in one breath

**The claim.** The decision belongs at the point where the shell has finished
deciding what the text meant — after expansion, after substitution, after the
path lookup — because that is the first moment the thing being judged is an
action rather than a spelling. `docs/design/sandboxing.md` states the same
thing from the other side, and was written before anyone here had heard of
GuardFall:

> this refuses what the shell itself does, and it is the only thing that can,
> because `eval` walks around anything applied from outside

**The limit, which has to be said in the same breath as the claim.** This
**contains the shell, not the process tree**. A program the shell was allowed
to start makes its own system calls and nothing here sees them. So the honest
form of the claim is *the decision point belongs inside the shell, and it
composes with an OS sandbox* — never *this contains an agent*. Overstating it
would be found out in a day and would cost more than it earns.

Both halves are measured below, with controls, because a refusal with no
control beside it cannot be told from a shell that fell over.

## What the resolved-exec decision buys

Measured 2026-09-21 through `bash --policy`, on a policy that is
`default allow` plus `deny exec /usr/bin/curl`. The command is spelled six
ways; the gate is asked about the resolved path each time.

| what is run | result |
| --- | --- |
| `curl --version` | `exec: refused: /usr/bin/curl` |
| `c=curl; $c --version` | `exec: refused: /usr/bin/curl` |
| `$(echo curl) --version` | `exec: refused: /usr/bin/curl` |
| `c""url --version` | `exec: refused: /usr/bin/curl` |
| `eval "$(echo Y3VybCAtLXZlcnNpb24= \| base64 -d)"` | `exec: refused: /usr/bin/curl` |
| `$(echo echo) allowed-ok` — **control** | `allowed-ok` |

Every row but the last is a spelling that defeats a text filter, and the last
row is the one that makes the others mean something: the same indirection
through an *allowed* program runs. The gate is deciding on what the shell
resolved, not on whether the text looked indirect.

The `eval` row is the one worth dwelling on. Text that only becomes a command
inside the shell is exactly what a filter in front of the shell never sees —
and it is the same route by which a policy applied from outside is walked
around.

## The scope limit, measured

Same binary, same kind of policy: `default allow` plus
`deny read <dir>/secret/**`.

| what is run | result |
| --- | --- |
| `read x < secret/s.txt` — the shell's own open | `open: refused: …/secret/s.txt`, and `x` is empty |
| `cat secret/s.txt` — a child's open | **prints `TOPSECRET`** |
| `read x < ok.txt` — **control**, an allowed file | `got=OKDATA` |
| `read x < secret/s.txt` with no policy — **control** | `got=TOPSECRET` |

The second row is the limit, stated as a measurement rather than as a caveat.
Granting exec of `/bin/cat` is `allow read /**` spelled less obviously,
which is why the policy grammar makes you write `allow exec-unconfined
/bin/cat` and refuses the shorter spelling.

The sharpest form of it is the handoff to another interpreter, which is also
one of GuardFall's own bypasses:

| what is run | result |
| --- | --- |
| `echo <base64> \| base64 -d \| /bin/sh` | **runs `curl`** — the child shell is outside the boundary |
| the same, with `deny exec /bin/sh` beside the `curl` rule | `exec: refused: /bin/sh` |

So the boundary holds where it is drawn, and a policy that allows an
interpreter has allowed everything that interpreter can do. That is the
sharpest footgun in the format and there is no way for a parser to detect it:
an allowlisted binary is a name, and what a name can do is not knowable from
here.

## What composes with it

Containing a running child needs the operating system — `seccomp`, Landlock, a
sandbox profile, a container — and this substrate does not own OS backends.
What it owns is the decision point, and a backend that wants one has it:
`interp.Gate` is `Allow(ctx, Action) Decision`, and nothing constrains how the
answer is reached. A `Gate` may consult the kernel, ask a supervisor, or block
on a person.

An OS sandbox and this are not alternatives and the composition is not
redundant. The OS sees a system call and no idea which line of script asked
for it; the shell sees the line of script and cannot follow a child. Each is
blind exactly where the other looks.

## Why the gate is believed here

The argument above is worth no more than the enforcement under it, and the
enforcement is checked by instruments that were built to be able to fail.

- **`make sandbox`** enumerates the *routes* a script has of reaching the
  filesystem — redirection in each form, `source`, a glob, a probe, an exec, a
  signal, and every module builtin that opens or changes a file — rather than
  the rules. Every escape this repository has had arrived somewhere a gate was
  not: `sysopen` and `zsystem flock` opened files with none (#1805), `autoload`
  read and then ran one (#1812), every mutating `zsh/files` builtin changed the
  filesystem with nothing consulted (#1819). None is expressible as a
  redirection.
- **No row is decided by one run.** A route is run ungated (it must *work*, or
  it is measuring nothing), denied (it must fail) and allowed (it must work
  again). A shell that did nothing would otherwise score perfect, which has
  happened here.
- **A route the shell cannot take yet is a ledger entry, not a skip**, because
  those are the rows that become escapes the day the feature lands. The ledger
  is empty today, and three features emptied it by bringing their gate in the
  same change.
- **The instrument was checked against a shell known to be broken.** Pointed at
  the commit before #1821 it reports the eleven escapes that change fixed;
  pointed at the commit after, none.
- **`make conformance-gated`** runs the whole corpus twice through the same
  binary, once under a policy, and reports what answered differently — so the
  boundary is exercised by fourteen hundred cases written for other reasons
  entirely, by people who were not thinking about it.
- **`make wild-run-contained`** runs every shell script installed on the
  machine under a policy confining writes to the directory the run was given.
  Its first run found that "a directory of its own" had never been a
  containment claim: three of the 252 scripts write to a temporary file they
  name themselves.

`docs/design/sandboxing.md` is the mechanism, the policy grammar and the event
schema. This page is the argument; that one is the specification, and where the
two disagree the specification is right.

## How somebody actually gets there

A published argument whose setup step is undocumented would be worse than not
publishing, and that objection was live on #1305 until #1334 and #2148 closed
it. `--policy` and `--audit` are read by `driver`, so all six binaries carry
them and no wrapper script is needed; the measurements on this page were taken
through `bash --policy`, not through a `-dialect` flag on `cmd/sh`.

`docs/install.md` § *Using this as a coding agent's shell* is the two-step
version: point `$SHELL` at one of these binaries, and put the policy in
`$SHELL` itself.

Two properties of the arrangement matter more than the convenience:

**A policy is never discovered.** No environment variable, no dotfile. It comes
from `--policy` and nowhere else, because a policy that could be named by the
environment could be replaced by anything that can set the environment —
including the script being sandboxed, on its way to invoking a nested shell.

**A refusal is a side effect that did not happen**, not a message. Every
instrument here asserts on the file not being there afterwards; a shell that
printed `refused` and wrote the file anyway would pass a check written the
other way.

## What is deliberately not claimed

- **Nothing about the network.** The gate has no network action kind, so a
  policy cannot express one, and a format that accepted `allow net` today would
  be accepting a rule nothing consults.
- **Nothing per-command.** Rules match actions, not the command that caused
  them. "Let `git` read `/srv` but not `curl`" is not expressible: once an exec
  has happened the child is outside the boundary entirely.
- **No prompting.** `Decision` is `Allow` or `Deny` and has no `Ask`. A
  question needs somewhere to ask it, which a library does not have; an
  embedder that has one writes a `Gate` that blocks on its own interface.
- **No claim about agents that do not run this shell.** GuardFall's finding is
  about eleven programs, none of which is this one, and nothing here has been
  run against them. What is claimed is the shape of the answer, not a fix
  shipped to anybody.

## Publication

Not done, and not to be done as a side effect of this page existing. Writing
the argument down and putting it in front of people are two decisions, and only
the first has been taken. Anything that goes out carries the scope limit in the
same breath as the claim, or it is not this argument.
