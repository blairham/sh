// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

// lowDescriptorCeiling is 56 on Linux, so that the hold never grows the
// process's descriptor table (#6396).
//
// The kernel starts a process with room for 64 descriptors and grows the table
// the first time one at or above 64 is taken. When the table is shared between
// threads — and every Go process's is, by the time a package initializer runs
// — the growth waits out an RCU grace period before the call returns. Measured
// in the golang:1.26.9 image: the one F_DUPFD that landed on 64 took 8 to 13
// ms of wall time and every other took about 0.1µs, and the runtime's idle
// threads kept waking for as long as the main one was blocked, which is where
// the CPU went.
//
// So the hold, and everything the runtime opens above it, has to end at 63.
// The runtime lands four numbers past the ceiling — openRuntimeDescriptors'
// pipe on the first two, then the poller's epoll and eventfd, which are the
// two that stay — so 59 is the highest ceiling that fits, and 60 does not:
// the eventfd lands on 64. Measured with /proc's FDSize after `-c` in that
// image, 99 and 60 leave a 128-entry table and 59 down to 56 leave 64.
//
// 56 rather than 59 is three numbers of room for a descriptor the caller
// handed over just above the ceiling, which pushes the runtime up by one each,
// and costs nothing measurable: 59 against 56 is −0.001 ms, faster in 51 of
// 100 pairs. Against the old 99, paired over 300 spawns of `-c 'echo ok'`
// from the container's own filesystem, CPU went from 2.313 to 1.947 ms
// (faster in 268 of 300) and wall from 12.0 to 1.7 ms.
//
// What it costs a script is two numbers, not 57 to 99: only the descriptors
// the runtime took are unplaceable, and every other number above the ceiling
// is placed as before. In that image they are 59 and 60. Everything a portable
// script can name, and everything `{v}` allocates, is still below 56.
const lowDescriptorCeiling = 56
