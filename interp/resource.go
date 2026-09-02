// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Resource names a limit `ulimit` can read or change.
//
// Named here rather than passed as the operating system's own RLIMIT_ number,
// so that this package says what it means and the caller does the translating
// — the same division the other process-state hooks use. It also keeps the
// constants out of a package that has no business importing syscall for them.
type Resource int

// The resources the panel's `ulimit` can address. Every shell in the panel
// spells each of these with the same letter, which is why the letters are not
// a dialect question; *which* letters a shell has is, and two of them differ.
const (
	// ResourceCore is `-c`, the largest core file. In blocks.
	ResourceCore Resource = iota
	// ResourceData is `-d`, the data segment. In kilobytes.
	ResourceData
	// ResourceFileSize is `-f`, the largest file this shell may write. In
	// blocks, and the block is the axis: 1024 bytes in bash and 512 in the
	// other three. Measured by writing until the kernel objected.
	ResourceFileSize
	// ResourceLockedMemory is `-l`. In kilobytes.
	ResourceLockedMemory
	// ResourceResidentSet is `-m`. In kilobytes. Absent from zsh.
	ResourceResidentSet
	// ResourceOpenFiles is `-n`, a count.
	ResourceOpenFiles
	// ResourceStack is `-s`. In kilobytes, unanimously — measured at exactly
	// 1024 against the raw limit, in every shell.
	ResourceStack
	// ResourceCPUTime is `-t`, in seconds.
	ResourceCPUTime
	// ResourceProcesses is `-u`, a count. Absent from dash.
	ResourceProcesses
	// ResourceAddressSpace is `-v`. In kilobytes.
	ResourceAddressSpace
)

// RlimitInfinity is "no limit", the value a shell prints as `unlimited` and
// reads back from that word.
//
// Its own constant rather than the operating system's RLIM_INFINITY, for the
// reason Resource is: this package names what it means and the caller
// translates. A caller whose idea of "no limit" is a different number maps it
// at the hook, which is also what lets a test double use an ordinary one.
const RlimitInfinity = int64(-1)

// resourceLetter is the option letter each resource is written with, and the
// scale its number is in.
type resourceLetter struct {
	letter byte
	res    Resource
	// scale is what one unit of the printed number is worth in the
	// operating system's own terms. Zero means the block scale, which is a
	// dialect question rather than a fixed number.
	scale int64
}

const kilobyte = 1024

var resourceLetters = []resourceLetter{
	{'c', ResourceCore, 0},
	{'d', ResourceData, kilobyte},
	{'f', ResourceFileSize, 0},
	{'l', ResourceLockedMemory, kilobyte},
	{'m', ResourceResidentSet, kilobyte},
	{'n', ResourceOpenFiles, 1},
	{'s', ResourceStack, kilobyte},
	{'t', ResourceCPUTime, 1},
	{'u', ResourceProcesses, 1},
	{'v', ResourceAddressSpace, kilobyte},
}
