# SPDX-FileCopyrightText: 2026 Blair Hamilton
# SPDX-License-Identifier: Apache-2.0

# One `FROM scratch` image per shell: the static binary, two files, two empty
# directories, and nothing else. #5185.
#
# The point is an image that has no userland and still has a shell — either run
# directly, or lifted into somebody else's distroless image with
#
#     COPY --from=ghcr.io/blairham/sh/scratch/bash:latest /bin/bash /bin/bash
#
# Every binary here is `CGO_ENABLED=0` and imports nothing outside the standard
# library, so there is no loader, no libc and no certificate store to carry.
# Measured in exactly such an image on 2026-09-29: `-c` runs, the interactive
# editor draws a prompt and edits a line over a pty with **no terminfo
# database on disk**, here-documents and `<(…)` work because neither needs a
# temporary file, and `> /dev/null` works because the runtime mounts `/dev`.
#
# What is deliberately **not** here:
#
#   - No zoneinfo. `TZ` is inert and every time is UTC — 3.5MB of database
#     against a 10MB binary is the wrong trade for a debugging shell, and the
#     `printf '%(…)T'` that reads it says `UTC` rather than failing.
#   - No certificate store. Nothing in a shell opens a TLS connection.
#   - No `ca-certificates`, no `/usr/bin`, no coreutils. `ls` is not a builtin
#     in any of these dialects, so an external command is `command not found`
#     until something else in the image provides one. That is the whole idea.

# The assembly runs on the **build** platform on purpose: it is `mkdir`, `ln`
# and two `printf`s, and nothing target-architecture is executed here — so a
# cross build needs no emulation. The binary is copied in below and only ever
# runs in the image the release smoke-tests.
FROM --platform=$BUILDPLATFORM busybox:1.37-uclibc@sha256:31b72538c3019a20336b04c4cebcd47c36511d0b45aa76666804c2845055bbd3 AS base

# The four gaps a bare `scratch` leaves, measured against the image with the
# binary alone in it:
#
#   - no `/tmp`, so `echo z > /tmp/x` is `No such file or directory`. Sticky
#     and world-writable, as every other `/tmp` is, so the image still works
#     under `--user`.
#   - no `/root`, so `$HOME` has nowhere to point and the history file has
#     nowhere to land.
#   - no `/etc/group`.
#   - no `/etc/passwd`, which is what makes `~root` come back as the two
#     characters written rather than a directory. It is written per shell
#     below, because root's shell field names the shell this image carries.
RUN set -eu; \
    mkdir -p /rootfs/bin /rootfs/etc; \
    mkdir -m 0755 /rootfs/root; \
    mkdir -m 1777 /rootfs/tmp; \
    printf 'root:x:0:\nnogroup:x:65534:\n' > /rootfs/etc/group

# One place that finishes a rootfs, rather than the same three lines under each
# of the six shells. The `ls` assertion is what makes a mismatch between the
# build's `--target` and the binary it was handed fail here instead of
# producing an image whose entrypoint does not exist.
COPY <<'EOF' /assemble
#!/bin/sh
# usage: /assemble <shell-name>
set -eu
name=$1
[ "$(ls /rootfs/bin)" = "$name" ] || {
	echo "expected /rootfs/bin to hold $name alone, holds: $(ls /rootfs/bin)" >&2
	exit 1
}
# A second name for the same binary, and fidelity rather than a convenience:
# driver.PosixNamed keys on the basename of argv[0], so this *is* the shell in
# POSIX mode, which is what real bash does when it is called `sh`. It also
# makes the image usable as a base, since `RUN` in a derived Dockerfile
# resolves `/bin/sh`.
ln -s "$name" /rootfs/bin/sh
printf 'root:x:0:0:root:/root:/bin/%s\n' "$name" > /rootfs/etc/passwd
printf 'nobody:x:65534:65534:nobody:/:/bin/sh\n' >> /rootfs/etc/passwd
EOF
RUN chmod 0755 /assemble

# Six shells, three stages each. The name is written literally rather than
# passed in, so `--target=bash` against a context carrying another shell's
# binary fails at the `COPY` below — a build error rather than a broken image.
#
# `ENTRYPOINT` is the reason each shell needs a stage of its own at all: its
# exec form takes no `ARG`, and the path has to be the real name for `$0` and
# for POSIX mode to read the way they do outside a container.

FROM base AS r-sh
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/sh /rootfs/bin/sh
RUN set -eu; \
    printf 'root:x:0:0:root:/root:/bin/sh\n' > /rootfs/etc/passwd; \
    printf 'nobody:x:65534:65534:nobody:/:/bin/sh\n' >> /rootfs/etc/passwd

FROM scratch AS sh
COPY --from=r-sh /rootfs/ /
ENV HOME=/root
ENTRYPOINT ["/bin/sh"]

FROM base AS r-bash
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/bash /rootfs/bin/bash
RUN /assemble bash

FROM scratch AS bash
COPY --from=r-bash /rootfs/ /
ENV HOME=/root
ENTRYPOINT ["/bin/bash"]

FROM base AS r-zsh
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/zsh /rootfs/bin/zsh
RUN /assemble zsh

FROM scratch AS zsh
COPY --from=r-zsh /rootfs/ /
ENV HOME=/root
ENTRYPOINT ["/bin/zsh"]

FROM base AS r-ksh
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/ksh /rootfs/bin/ksh
RUN /assemble ksh

FROM scratch AS ksh
COPY --from=r-ksh /rootfs/ /
ENV HOME=/root
ENTRYPOINT ["/bin/ksh"]

FROM base AS r-dash
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/dash /rootfs/bin/dash
RUN /assemble dash

FROM scratch AS dash
COPY --from=r-dash /rootfs/ /
ENV HOME=/root
ENTRYPOINT ["/bin/dash"]

FROM base AS r-ash
ARG TARGETOS TARGETARCH
COPY ${TARGETOS}/${TARGETARCH}/ash /rootfs/bin/ash
RUN /assemble ash

FROM scratch AS ash
COPY --from=r-ash /rootfs/ /
ENV HOME=/root
ENTRYPOINT ["/bin/ash"]
