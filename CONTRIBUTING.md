# Contributing

Thank you for considering a contribution. Two things here are unusual
and non-negotiable, so please read both before opening a pull request.

## 1. The clean-room rule

This is an **independent implementation**. `CLEANROOM.md` is binding:

> Never read another shell implementation's source code while writing
> this one.

Not mvdan.cc/sh, not bash, not dash, not zsh, not busybox — and not to
"check how they did it". Behavior is learned from POSIX, from vendor
manuals, and from running real shell binaries as oracles. It is written
down in `docs/spec/`, and code is written from the spec.

A contribution that came from reading an implementation cannot be
accepted, however thoroughly it was reworded. Re-expressing source you
have read produces a derivative work, and one such commit would
compromise the licensing of the whole project.

If you are stuck, the escalation path is in `CLEANROOM.md` — it ends at
an oracle run, never at someone else's source.

## 2. The Contributor Licence Agreement

Contributions require a signed CLA. A bot will prompt you on your first
pull request; signing takes one comment.

**Why.** The project may need to offer different licensing terms in the
future. That is only possible if one party can license the whole work,
and copyright in a contribution stays with its author unless licensed
onward. Without a CLA, a single contribution would permanently remove
that option — not because the contribution was unwelcome, but because
nobody could relicense it later.

The CLA does **not** take your copyright. You keep it. You grant a
licence broad enough to include sublicensing, and you affirm the work is
your own.

The agreement text is in `CLA.md`.

## Practical

- Work on a branch and open a pull request; do not commit to `main`.
- Commit messages explain the **why**, not a restatement of the diff.
- No AI-attribution trailers in commit messages or pull request bodies.
- Every `.go` file carries the two-line SPDX header — see `AGENTS.md`.
  CI fails without it.
- `make check` must pass. Lint runs in CI.
- New behavior needs a spec entry in `docs/spec/` with a citation: a
  POSIX section, a manual section, or a recorded oracle run.
