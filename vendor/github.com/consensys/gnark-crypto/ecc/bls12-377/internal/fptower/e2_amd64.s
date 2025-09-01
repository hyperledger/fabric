// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"

#define REDUCE(ra0, ra1, ra2, ra3, ra4, ra5, rb0, rb1, rb2, rb3, rb4, rb5, q0, q1, q2, q3, q4, q5) \
	MOVQ    ra0, rb0; \
	SUBQ    q0, ra0;  \
	MOVQ    ra1, rb1; \
	SBBQ    q1, ra1;  \
	MOVQ    ra2, rb2; \
	SBBQ    q2, ra2;  \
	MOVQ    ra3, rb3; \
	SBBQ    q3, ra3;  \
	MOVQ    ra4, rb4; \
	SBBQ    q4, ra4;  \
	MOVQ    ra5, rb5; \
	SBBQ    q5, ra5;  \
	CMOVQCS rb0, ra0; \
	CMOVQCS rb1, ra1; \
	CMOVQCS rb2, ra2; \
	CMOVQCS rb3, ra3; \
	CMOVQCS rb4, ra4; \
	CMOVQCS rb5, ra5; \

#define REDUCE_NOGLOBAL(ra0, ra1, ra2, ra3, ra4, ra5, rb0, rb1, rb2, rb3, rb4, rb5, scratch0) \
	MOVQ    ra0, rb0;            \
	MOVQ    $const_q0, scratch0; \
	SUBQ    scratch0, ra0;       \
	MOVQ    ra1, rb1;            \
	MOVQ    $const_q1, scratch0; \
	SBBQ    scratch0, ra1;       \
	MOVQ    ra2, rb2;            \
	MOVQ    $const_q2, scratch0; \
	SBBQ    scratch0, ra2;       \
	MOVQ    ra3, rb3;            \
	MOVQ    $const_q3, scratch0; \
	SBBQ    scratch0, ra3;       \
	MOVQ    ra4, rb4;            \
	MOVQ    $const_q4, scratch0; \
	SBBQ    scratch0, ra4;       \
	MOVQ    ra5, rb5;            \
	MOVQ    $const_q5, scratch0; \
	SBBQ    scratch0, ra5;       \
	CMOVQCS rb0, ra0;            \
	CMOVQCS rb1, ra1;            \
	CMOVQCS rb2, ra2;            \
	CMOVQCS rb3, ra3;            \
	CMOVQCS rb4, ra4;            \
	CMOVQCS rb5, ra5;            \

TEXT ·addE2(SB), $8-24
	MOVQ x+8(FP), AX
	MOVQ 0(AX), CX
	MOVQ 8(AX), BX
	MOVQ 16(AX), SI
	MOVQ 24(AX), DI
	MOVQ 32(AX), R8
	MOVQ 40(AX), R9
	MOVQ y+16(FP), DX
	ADDQ 0(DX), CX
	ADCQ 8(DX), BX
	ADCQ 16(DX), SI
	ADCQ 24(DX), DI
	ADCQ 32(DX), R8
	ADCQ 40(DX), R9

	// reduce element(CX,BX,SI,DI,R8,R9,) using temp registers (R11,R12,R13,R14,R15,s0-8(SP),R10)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R8,R9,R11,R12,R13,R14,R15,s0-8(SP),R10)

	MOVQ res+0(FP), R11
	MOVQ CX, 0(R11)
	MOVQ BX, 8(R11)
	MOVQ SI, 16(R11)
	MOVQ DI, 24(R11)
	MOVQ R8, 32(R11)
	MOVQ R9, 40(R11)
	MOVQ 48(AX), CX
	MOVQ 56(AX), BX
	MOVQ 64(AX), SI
	MOVQ 72(AX), DI
	MOVQ 80(AX), R8
	MOVQ 88(AX), R9
	ADDQ 48(DX), CX
	ADCQ 56(DX), BX
	ADCQ 64(DX), SI
	ADCQ 72(DX), DI
	ADCQ 80(DX), R8
	ADCQ 88(DX), R9

	// reduce element(CX,BX,SI,DI,R8,R9,) using temp registers (R13,R14,R15,R10,AX,DX,R12)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R8,R9,R13,R14,R15,R10,AX,DX,R12)

	MOVQ CX, 48(R11)
	MOVQ BX, 56(R11)
	MOVQ SI, 64(R11)
	MOVQ DI, 72(R11)
	MOVQ R8, 80(R11)
	MOVQ R9, 88(R11)
	RET

TEXT ·doubleE2(SB), $8-16
	MOVQ res+0(FP), DX
	MOVQ x+8(FP), AX
	MOVQ 0(AX), CX
	MOVQ 8(AX), BX
	MOVQ 16(AX), SI
	MOVQ 24(AX), DI
	MOVQ 32(AX), R8
	MOVQ 40(AX), R9
	ADDQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8
	ADCQ R9, R9

	// reduce element(CX,BX,SI,DI,R8,R9,) using temp registers (R11,R12,R13,R14,R15,s0-8(SP),R10)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R8,R9,R11,R12,R13,R14,R15,s0-8(SP),R10)

	MOVQ CX, 0(DX)
	MOVQ BX, 8(DX)
	MOVQ SI, 16(DX)
	MOVQ DI, 24(DX)
	MOVQ R8, 32(DX)
	MOVQ R9, 40(DX)
	MOVQ 48(AX), CX
	MOVQ 56(AX), BX
	MOVQ 64(AX), SI
	MOVQ 72(AX), DI
	MOVQ 80(AX), R8
	MOVQ 88(AX), R9
	ADDQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8
	ADCQ R9, R9

	// reduce element(CX,BX,SI,DI,R8,R9,) using temp registers (R12,R13,R14,R15,R10,s0-8(SP),R11)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R8,R9,R12,R13,R14,R15,R10,s0-8(SP),R11)

	MOVQ CX, 48(DX)
	MOVQ BX, 56(DX)
	MOVQ SI, 64(DX)
	MOVQ DI, 72(DX)
	MOVQ R8, 80(DX)
	MOVQ R9, 88(DX)
	RET

TEXT ·subE2(SB), NOSPLIT, $0-24
	XORQ    R15, R15
	MOVQ    x+8(FP), R8
	MOVQ    0(R8), AX
	MOVQ    8(R8), DX
	MOVQ    16(R8), CX
	MOVQ    24(R8), BX
	MOVQ    32(R8), SI
	MOVQ    40(R8), DI
	MOVQ    y+16(FP), R8
	SUBQ    0(R8), AX
	SBBQ    8(R8), DX
	SBBQ    16(R8), CX
	SBBQ    24(R8), BX
	SBBQ    32(R8), SI
	SBBQ    40(R8), DI
	MOVQ    x+8(FP), R8
	MOVQ    $0x8508c00000000001, R9
	MOVQ    $0x170b5d4430000000, R10
	MOVQ    $0x1ef3622fba094800, R11
	MOVQ    $0x1a22d9f300f5138f, R12
	MOVQ    $0xc63b05c06ca1493b, R13
	MOVQ    $0x01ae3a4617c510ea, R14
	CMOVQCC R15, R9
	CMOVQCC R15, R10
	CMOVQCC R15, R11
	CMOVQCC R15, R12
	CMOVQCC R15, R13
	CMOVQCC R15, R14
	ADDQ    R9, AX
	ADCQ    R10, DX
	ADCQ    R11, CX
	ADCQ    R12, BX
	ADCQ    R13, SI
	ADCQ    R14, DI
	MOVQ    res+0(FP), R9
	MOVQ    AX, 0(R9)
	MOVQ    DX, 8(R9)
	MOVQ    CX, 16(R9)
	MOVQ    BX, 24(R9)
	MOVQ    SI, 32(R9)
	MOVQ    DI, 40(R9)
	MOVQ    48(R8), AX
	MOVQ    56(R8), DX
	MOVQ    64(R8), CX
	MOVQ    72(R8), BX
	MOVQ    80(R8), SI
	MOVQ    88(R8), DI
	MOVQ    y+16(FP), R8
	SUBQ    48(R8), AX
	SBBQ    56(R8), DX
	SBBQ    64(R8), CX
	SBBQ    72(R8), BX
	SBBQ    80(R8), SI
	SBBQ    88(R8), DI
	MOVQ    $0x8508c00000000001, R10
	MOVQ    $0x170b5d4430000000, R11
	MOVQ    $0x1ef3622fba094800, R12
	MOVQ    $0x1a22d9f300f5138f, R13
	MOVQ    $0xc63b05c06ca1493b, R14
	MOVQ    $0x01ae3a4617c510ea, R9
	CMOVQCC R15, R10
	CMOVQCC R15, R11
	CMOVQCC R15, R12
	CMOVQCC R15, R13
	CMOVQCC R15, R14
	CMOVQCC R15, R9
	ADDQ    R10, AX
	ADCQ    R11, DX
	ADCQ    R12, CX
	ADCQ    R13, BX
	ADCQ    R14, SI
	ADCQ    R9, DI
	MOVQ    res+0(FP), R8
	MOVQ    AX, 48(R8)
	MOVQ    DX, 56(R8)
	MOVQ    CX, 64(R8)
	MOVQ    BX, 72(R8)
	MOVQ    SI, 80(R8)
	MOVQ    DI, 88(R8)
	RET

TEXT ·negE2(SB), NOSPLIT, $0-16
	MOVQ  res+0(FP), DX
	MOVQ  x+8(FP), AX
	MOVQ  0(AX), BX
	MOVQ  8(AX), SI
	MOVQ  16(AX), DI
	MOVQ  24(AX), R8
	MOVQ  32(AX), R9
	MOVQ  40(AX), R10
	MOVQ  BX, AX
	ORQ   SI, AX
	ORQ   DI, AX
	ORQ   R8, AX
	ORQ   R9, AX
	ORQ   R10, AX
	TESTQ AX, AX
	JNE   l1
	MOVQ  AX, 0(DX)
	MOVQ  AX, 8(DX)
	MOVQ  AX, 16(DX)
	MOVQ  AX, 24(DX)
	MOVQ  AX, 32(DX)
	MOVQ  AX, 40(DX)
	JMP   l3

l1:
	MOVQ $0x8508c00000000001, CX
	SUBQ BX, CX
	MOVQ CX, 0(DX)
	MOVQ $0x170b5d4430000000, CX
	SBBQ SI, CX
	MOVQ CX, 8(DX)
	MOVQ $0x1ef3622fba094800, CX
	SBBQ DI, CX
	MOVQ CX, 16(DX)
	MOVQ $0x1a22d9f300f5138f, CX
	SBBQ R8, CX
	MOVQ CX, 24(DX)
	MOVQ $0xc63b05c06ca1493b, CX
	SBBQ R9, CX
	MOVQ CX, 32(DX)
	MOVQ $0x01ae3a4617c510ea, CX
	SBBQ R10, CX
	MOVQ CX, 40(DX)

l3:
	MOVQ  x+8(FP), AX
	MOVQ  48(AX), BX
	MOVQ  56(AX), SI
	MOVQ  64(AX), DI
	MOVQ  72(AX), R8
	MOVQ  80(AX), R9
	MOVQ  88(AX), R10
	MOVQ  BX, AX
	ORQ   SI, AX
	ORQ   DI, AX
	ORQ   R8, AX
	ORQ   R9, AX
	ORQ   R10, AX
	TESTQ AX, AX
	JNE   l2
	MOVQ  AX, 48(DX)
	MOVQ  AX, 56(DX)
	MOVQ  AX, 64(DX)
	MOVQ  AX, 72(DX)
	MOVQ  AX, 80(DX)
	MOVQ  AX, 88(DX)
	RET

l2:
	MOVQ $0x8508c00000000001, CX
	SUBQ BX, CX
	MOVQ CX, 48(DX)
	MOVQ $0x170b5d4430000000, CX
	SBBQ SI, CX
	MOVQ CX, 56(DX)
	MOVQ $0x1ef3622fba094800, CX
	SBBQ DI, CX
	MOVQ CX, 64(DX)
	MOVQ $0x1a22d9f300f5138f, CX
	SBBQ R8, CX
	MOVQ CX, 72(DX)
	MOVQ $0xc63b05c06ca1493b, CX
	SBBQ R9, CX
	MOVQ CX, 80(DX)
	MOVQ $0x01ae3a4617c510ea, CX
	SBBQ R10, CX
	MOVQ CX, 88(DX)
	RET
