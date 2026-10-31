// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// One trampoline per libSystem function crashreports_darwin.go calls, and the
// variable that holds its address. JMP is the same instruction on amd64 and
// arm64, so one file serves both.

#include "textflag.h"

TEXT taskselftrap<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_task_self_trap(SB)
GLOBL	·taskSelfTrapAddr(SB), RODATA, $8
DATA	·taskSelfTrapAddr(SB)/8, $taskselftrap<>(SB)

TEXT machportallocate<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_mach_port_allocate(SB)
GLOBL	·machPortAllocateAddr(SB), RODATA, $8
DATA	·machPortAllocateAddr(SB)/8, $machportallocate<>(SB)

TEXT machportinsertright<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_mach_port_insert_right(SB)
GLOBL	·machPortInsertRightAddr(SB), RODATA, $8
DATA	·machPortInsertRightAddr(SB)/8, $machportinsertright<>(SB)

TEXT machportmodrefs<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_mach_port_mod_refs(SB)
GLOBL	·machPortModRefsAddr(SB), RODATA, $8
DATA	·machPortModRefsAddr(SB)/8, $machportmodrefs<>(SB)

TEXT tasksetexceptionports<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_task_set_exception_ports(SB)
GLOBL	·taskSetExceptionPortsAddr(SB), RODATA, $8
DATA	·taskSetExceptionPortsAddr(SB)/8, $tasksetexceptionports<>(SB)

TEXT machmsgreceive<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_mach_msg_receive(SB)
GLOBL	·machMsgReceiveAddr(SB), RODATA, $8
DATA	·machMsgReceiveAddr(SB)/8, $machmsgreceive<>(SB)

TEXT machmsgsend<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_mach_msg_send(SB)
GLOBL	·machMsgSendAddr(SB), RODATA, $8
DATA	·machMsgSendAddr(SB)/8, $machmsgsend<>(SB)

TEXT machmsgdestroy<>(SB),NOSPLIT,$0-0
	JMP	shtestenv_mach_msg_destroy(SB)
GLOBL	·machMsgDestroyAddr(SB), RODATA, $8
DATA	·machMsgDestroyAddr(SB)/8, $machmsgdestroy<>(SB)
