# Architecture

A shell parser and interpreter in Go whose core language is the common
denominator of real shells, whose dialects are presets over a semantics
vector, and whose execution is observable and gateable from the inside.

Read `CLEANROOM.md` before writing code, and `docs/spec/` before
designing behaviour.

## The three layers

1. **Grammar** — a variant set on the parser. Constructs are additive:
   a dialect enables constructs the core does not have. See
   `spec/shell-matrix.md`.
2. **Semantics** — a vector of named switches for the places dialects
   disagree about identical syntax. See `spec/semantics.md`.
3. **Dialects** — named presets (`posix`, `core`, `bash`, `zsh`, `ksh`)
   that select a variant set and a semantics vector. A dialect is a
   *table of values*, never a code path.

A dialect is runtime state, re-read as execution proceeds — `set -o
posix` halfway through a script governs the rest of it, including the
parse of the rest of it. Parser configuration is therefore derived from
interpreter state on demand, never fixed at construction.

## Agent, AI and sandbox are core, not wrappers

These are first-class requirements, and the reason is structural rather
than aspirational.

**A policy enforced outside the interpreter does not reach inside it.**
`eval`, `source`, command substitution and subshells all re-enter the
interpreter with their own input. Anything that must hold universally —
a sandbox boundary, an audit trail, a permission prompt — must live at
the point of execution, or it is bypassed by the first `eval`. A shell
that sandboxes by wrapping the process it spawns has sandboxed nothing
that matters.

So the seams are interpreter-internal from line one.

**All three requirements are the same shape.** Sandboxing gates
execution. An agent protocol streams what happened and relays permission
questions. An AI assistant needs structured context about what ran and
what failed. Each is either *gating* an action or *observing* one, so
two primitives serve all three:

| primitive | question it answers | serves |
| --- | --- | --- |
| **gate** | may this action proceed — allow, deny, or ask? | sandbox policy; agent permission prompts |
| **event** | what just happened, structured? | agent streaming; AI context; audit; trace |

Every action that leaves the interpreter's own memory passes both:
process execution, file open, file stat, directory read, and each
redirect. That set is the syscall-shaped surface of a shell, and it is
the complete boundary.

**What lives here and what does not.** The substrate owns the gate, the
event stream, and the structured representation of an error — what
failed, its status, its output, its position in the source. It does not
own OS sandbox backends, protocol transports, or model providers. A
library that imports an AI SDK is a library nobody adopts; the seams are
here, the implementations sit above.

## The process model is decided now

Background jobs are **real process groups**, not goroutines.

This is recorded as a decision because it is the expensive kind. A shell
built on goroutine-backed jobs cannot later give honest answers for job
control, signal delivery, terminal ownership or `set -m`, and cannot
place a sandbox boundary or a permission gate around a process group
that does not exist. Retrofitting it means rewriting execution.

## What is deliberately not here

- **fish.** fish is not a Bourne descendant and shares no grammar below
  the command name — assignment is a command, blocks end with `end`,
  every variable is a list. There is no switch that turns this core into
  fish; it would be a separate front-end. The parser's own vocabulary
  reflects this: dialects are bash, ksh, mksh and zsh.
- **A bash-complete long tail.** Compatibility is measured and published,
  not claimed. The core is the common denominator; bash's corners belong
  to the bash dialect and arrive on evidence of demand.
