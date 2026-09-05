// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver

// RuntimeDescriptorsForTest is the census taken at initialization: the
// descriptor numbers the Go runtime answered for once the low range had been
// held and it had been made to open everything it opens lazily.
//
// Exposed because the property this package now has is not visible in any
// shell's output — a shell whose runtime sits on descriptor 5 behaves exactly
// like one whose runtime sits on 102 until a script parks on 5, and then it
// dies rather than misbehaving.
func RuntimeDescriptorsForTest() []int { return runtimeDescriptors }

// LowDescriptorCeilingForTest is the highest number the runtime is kept off.
func LowDescriptorCeilingForTest() int { return lowDescriptorCeiling }
