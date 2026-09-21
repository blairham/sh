# Reference images

`make suite` grades each dialect binary against the reference shell it claims
to be. A column whose reference is *whatever build the machine happens to
have* reports a fact about that machine: on `ubuntu-latest` the bash column
read 62/73 strict against the runner's 5.2.21 and 70/73 against the 5.3.20 its
cases were measured with, and eight of the eleven differences were the version
skew rather than anything this shell does. The harness prints `WRONG BUILD`
over such a figure, which is honest and still leaves a number nobody can act
on, and `#2291`'s `strict 100%` bar cannot be met over one by any amount of
correct work.

So a column is **gated**: both shells run inside an image pinned by digest, on
one copy of the files, and the figure is the same on a runner and on a laptop.
Two of the four columns that could be gated already were, off the shelf —
`ash` through the oracle panel's Alpine (`#2263`) and `bash` through
`bash:5.3` (`#3797`). The other two are here, because **no public image is the
build their cases were measured against**:

| column | graded against | why an image had to be built |
| --- | --- | --- |
| `zsh` | `zsh 5.9.2` | every distribution image that has 5.9 reports exactly `zsh 5.9`, and 5.9 is not a substitute — it answers two of this column's own files differently |
| `dash` | `esc=4 pipefail-listed=n` | Debian's and Ubuntu's dash is patched, and the patch is the whole of this column's gap on a runner |

Each directory here is one image. Both shells in it are the reference; the
dialect binary is cross-compiled in beside it by
[`internal/suite`](../internal/suite/container.go), exactly as the `ash` route
has always worked.

## What is pinned, and what is not

**The published image's digest is the pin.** It is written down in exactly one
place, `internal/suite/own.go`, and nowhere in prose — a digest quoted in a
comment or an issue is stale the moment it is read, which `bash:5.3` proved by
moving twice in two days while `#3480` was being measured.

**The recipes here are not reproducible byte for byte, and that is why
publishing is deliberate.** `zsh-5.9.2` installs from sid, which moves; a
rebuild months from now would produce a different image under the same tag and
leave the digest a column is pinned to untagged. So the workflow publishes only
from a `refimages/` branch, never from a merge, and a pull request that
touches a recipe builds it without pushing.

What keeps a recipe honest instead of reproducible is that **each one asserts,
at build time, that what it has installed is the build the column names** — the
zsh image runs the two `kill` probes that separate 5.9.2 from 5.9, and the dash
image runs the column's own `AgainstProbe` verbatim. An image that is not the
reference fails to build rather than being published and discovered later
through a `WRONG BUILD` banner.

## Republishing one

    git switch -c refimages/<what>
    git push -u origin refimages/<what>

Read the index digest out of the run's summary, put it in the column's `Digest`
in `internal/suite/own.go`, and dispatch `CI` against the branch to read the
column before merging (`#3720`). Delete the `refimages/` branch afterwards;
it is a trigger rather than work.

The digest is an **index** digest, so one pin covers the amd64 runner and the
arm64 laptop. Do not delete an old version from the package after republishing:
a column pinned to it is still pulling it.

## zsh-5.9.2

Debian sid packages `zsh 5.9.2`, which is what this column names and what no
image ships preinstalled — measured 2026-09-20, after the three earlier sweeps
on `#3480` found only 5.9 anywhere.

5.9 is not close enough, and this is the measurement rather than a preference.
Over the column's 81 files under two builds on one machine, 79 are
byte-identical and two are not — `zsh/builtins.tests` and
`zsh/diagnostics.tests` — and both differences are `kill`:

| probe | 5.9.2 | 5.9 |
| --- | --- | --- |
| `kill -L` | accepted | refused; the usage line says `type kill -l for a list of signals` |
| `kill -l 160` | `160` | `32` |
| `kill -l 257` | `257` | `HUP` |

Two independent 5.9 builds — Apple's `/bin/zsh` and a Linux `zsh:5.9` image —
agree with each other against 5.9.2, so it is the release and not one vendor's
patch. Gating against 5.9 would have pinned the figure and recorded a claim the
measurement contradicts.

## dash-0.5.12

Upstream 0.5.12, built from the tarball, because no distribution ships an
unpatched one. Measured across five images on `#3480`: `debian:bookworm-slim`,
`debian:trixie-slim`, `ubuntu:24.04` and `ubuntu:22.04` all answer `esc=3`
where this column is graded against `esc=4`, trixie adding
`pipefail-listed=y`, and Alpine has no dash at all. Debian sid's is
`0.5.12-12+b1` and answers `esc=3 pipefail-listed=y` — the same upstream
version, patched.

The patch is not cosmetic. It is the whole of this column's gap on a runner —
67/67 strict on the build the cases were written beside against 64/67 there —
and the three files are `dash/printf.tests` (`printf 'a\eZ'` writes the escape
rather than the four characters), `dash/builtins.tests` (`privileged` in the
`set -o` listing, which upstream's table has not) and `dash/variables.tests`
(`LINENO`, present upstream and absent in Debian's).

The tarball is pinned by content rather than by URL: `deb.debian.org`'s
`dash_0.5.12.orig.tar.gz` and upstream's own `dash-0.5.12.tar.gz` are the same
241KB archive, byte for byte, measured 2026-09-20. The build replaces the
distribution's `/bin/dash` rather than sitting beside it, so the image holds
one dash and the lookup order cannot pick the wrong one.
