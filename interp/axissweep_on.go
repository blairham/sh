// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build shaxissweep

package interp

import "github.com/blairham/sh/internal/axismutate"

// axisMutate moves the one axis the sweep is asking about.
//
// Built only under `-tags shaxissweep`, which nothing ships. See
// interp/axissweep_off.go for what this is and why it exists, and
// internal/axissweep for the instrument that sets the variable.
//
// It takes its own copy and mutates that rather than the runner's vector, so a mutation cannot leak into a struct the caller owns — a subshell
// clone shares the vector by pointer, and writing through it would move the
// axis for a parent that had not asked.
func axisMutate(s *Semantics) *Semantics {
	c := *s
	axismutate.ApplyEnv(&c)
	return &c
}
