# Clean-room rules

This repository is an **independent implementation** of a POSIX-family
shell parser and interpreter in Go. It is not a fork, not a port, and
not a rewrite-in-place of any existing project. This document is the
rule set that keeps that claim true, and it is binding on every
contributor and every AI agent working in this tree.

Read it before writing code. It is short on purpose.

## Why this exists

There is a mature incumbent in this space (`mvdan.cc/sh`, BSD-3-Clause).
Its licence is permissive and using it would have been legal and cheap —
the obligation is a retained notice. **We are not writing this to escape
that notice.** We are writing it because we want a substrate whose core
is the common denominator of real shells and whose dialects are
first-class presets over a semantics vector, and that is an
architectural decision that cannot be retrofitted into a codebase built
bash-first.

Stating the motive matters. A rewrite undertaken to shed attribution
invites the question of whether it is really independent. A rewrite
undertaken for architecture does not, provided the rules below hold.

## The rule

> **Never read another shell implementation's source code while writing
> this one.**

Not to "check how they did it". Not to "get unstuck". Not once.

Re-expressing source you have just read produces work derived from its
structure, sequence and organisation. That is a derivative work even
when no line is copied verbatim, and it is the single mistake that
would undo the entire effort.

## Two phases, and a wall between them

Independence comes from separating *learning what a shell does* from
*writing code that does it*.

| phase | may consult | produces |
| --- | --- | --- |
| **spec** | POSIX XCU, the bash/ksh/zsh/dash manuals, observed behaviour of real shell binaries, standards mailing lists, bug trackers (a bug *report* describes behaviour) | a behavioural spec, in our own words, under `docs/spec/` |
| **implement** | **only `docs/spec/`** and the Go standard library | code |

`docs/spec/` is the wall. Anything that crosses it must be a
**behavioural fact**, never someone else's **expression** of that fact.

If you are implementing and you find the spec inadequate: stop, go back
to the spec phase, extend the spec from the specification or from an
oracle run, then resume. Do not resolve the gap by reading an
implementation.

## Green list — consult freely

- **POSIX.1 / XCU, Shell Command Language.** The normative source.
  Grammar and behaviour are not copyrightable; this is the primary input.
- **The bash, ksh, mksh, dash and zsh reference manuals.** Documentation
  describing behaviour.
- **Observed behaviour of real shell binaries.** Running `bash -c '...'`
  and recording what happens produces *facts*. This is our best source
  and it is entirely clean. See `docs/spec/oracle.md`.
- **Ideas, architecture and algorithms.** 17 U.S.C. §102(b): copyright
  does not extend to ideas, procedures or methods of operation. A
  variant bitset, an option vector, a recursive-descent parser are all
  ideas.
- **Public interface shape** — that a shell parser exposes something
  called a parser producing something called a syntax tree. Names and
  interfaces are facts about the domain.
- **Anything the author of this repository wrote**, including obi's
  `internal/compat` corpus and its builtins.

## Red list — never, while working in this tree

- **Source code of any other shell implementation.** `mvdan.cc/sh`,
  bash, dash, zsh, ksh, busybox ash — the C ones are as off-limits as
  the Go one, and bash's is GPLv3, which is worse.
- **Another project's test files or testdata.** These are expression,
  and they are exactly as protected as the implementation. Our tests are
  generated from oracle runs instead.
- **Vendored, copied or pasted code of any provenance**, however
  reformatted, renamed or "rewritten line by line".
- **AI-generated code produced by prompting a model to reproduce, port,
  translate or imitate a specific existing implementation.** Asking for
  "a POSIX word-splitting routine" is fine; asking for "mvdan's
  word-splitting routine in different words" is not.

## Tests come from oracles, not from other people's suites

We prove compatibility by **differential testing against real shell
binaries**, never by importing anyone's test corpus.

A case is a snippet plus the observed output and exit status of a real
`bash` / `dash` / `zsh` / `ksh`. The snippet is ours; the output is a
fact about that binary. This is clean, and it is also simply better
evidence, because it measures the shell people actually run rather than
what a third party believed it did.

Where a third-party suite must be run — bash's own `tests/`, which is
GPLv3 — it is **fetched at test time and never committed**. Committing
it would relicense this repository by accident.

## If you think you need to look

You do not. The escalation path is:

1. Re-read the POSIX text for the construct.
2. Ask the oracle: write the snippet, run it against four shells, record
   what happened.
3. Read the vendor manual.
4. Write the behaviour down in `docs/spec/`, then implement from that.

Step 2 answers almost everything, because the question is nearly always
"what does it actually do", and an implementation is a worse answer to
that than the binary itself.

## Provenance hygiene

- Every commit is our own work. If you cannot say where a construct's
  behaviour was learned, do not commit it.
- `docs/spec/` cites its sources: a POSIX section number, a manual
  section, or an oracle run. Citations are to *specifications and
  observations*, never to source files.
- No file in this repository carries another project's copyright header,
  because no file in this repository derives from another project.
- `LICENSE` is MIT and stands alone. There is no `LICENSE-THIRD-PARTY`
  for the substrate, and if one ever becomes necessary the rule above
  has been broken.
