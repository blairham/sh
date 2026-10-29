// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package driver_test

// nameableCeiling is the highest descriptor this test insists the runtime
// stays above, and it is exactly the ceiling: see lowfds_unix_test.go's
// TestTheRuntimeHoldsNoDescriptorAScriptCanName.
//
// Written out rather than read from the package, and that is the whole point
// of it being here: a test that took the ceiling from the code it is checking
// would follow the ceiling down. Lowering it to 9 — which is all the panel can
// name, and a defensible-looking number — would leave the runtime on 10, 11
// and 12, which is where bash's own `exec {v}>f` allocates from, and every
// assertion in that file would still pass. This one would not.
//
// 56 off Linux as on it (#6491): see lowfds_unixother.go.
const nameableCeiling = 56

// replacementProbes are the numbers the replacement test parks on: every
// single digit, the first number bash's `{v}` allocates, and the ceiling.
var replacementProbes = []int{3, 4, 5, 6, 7, 8, 9, 10, 56}
