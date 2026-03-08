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
	MOVQ    $0xb9feffffffffaaab, R9
	MOVQ    $0x1eabfffeb153ffff, R10
	MOVQ    $0x6730d2a0f6b0f624, R11
	MOVQ    $0x64774b84f38512bf, R12
	MOVQ    $0x4b1ba7b6434bacd7, R13
	MOVQ    $0x1a0111ea397fe69a, R14
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
	MOVQ    $0xb9feffffffffaaab, R10
	MOVQ    $0x1eabfffeb153ffff, R11
	MOVQ    $0x6730d2a0f6b0f624, R12
	MOVQ    $0x64774b84f38512bf, R13
	MOVQ    $0x4b1ba7b6434bacd7, R14
	MOVQ    $0x1a0111ea397fe69a, R9
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
	MOVQ $0xb9feffffffffaaab, CX
	SUBQ BX, CX
	MOVQ CX, 0(DX)
	MOVQ $0x1eabfffeb153ffff, CX
	SBBQ SI, CX
	MOVQ CX, 8(DX)
	MOVQ $0x6730d2a0f6b0f624, CX
	SBBQ DI, CX
	MOVQ CX, 16(DX)
	MOVQ $0x64774b84f38512bf, CX
	SBBQ R8, CX
	MOVQ CX, 24(DX)
	MOVQ $0x4b1ba7b6434bacd7, CX
	SBBQ R9, CX
	MOVQ CX, 32(DX)
	MOVQ $0x1a0111ea397fe69a, CX
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
	MOVQ $0xb9feffffffffaaab, CX
	SUBQ BX, CX
	MOVQ CX, 48(DX)
	MOVQ $0x1eabfffeb153ffff, CX
	SBBQ SI, CX
	MOVQ CX, 56(DX)
	MOVQ $0x6730d2a0f6b0f624, CX
	SBBQ DI, CX
	MOVQ CX, 64(DX)
	MOVQ $0x64774b84f38512bf, CX
	SBBQ R8, CX
	MOVQ CX, 72(DX)
	MOVQ $0x4b1ba7b6434bacd7, CX
	SBBQ R9, CX
	MOVQ CX, 80(DX)
	MOVQ $0x1a0111ea397fe69a, CX
	SBBQ R10, CX
	MOVQ CX, 88(DX)
	RET

TEXT ·mulNonResE2(SB), NOSPLIT, $0-16
	XORQ    R15, R15
	MOVQ    x+8(FP), R14
	MOVQ    0(R14), AX
	MOVQ    8(R14), DX
	MOVQ    16(R14), CX
	MOVQ    24(R14), BX
	MOVQ    32(R14), SI
	MOVQ    40(R14), DI
	SUBQ    48(R14), AX
	SBBQ    56(R14), DX
	SBBQ    64(R14), CX
	SBBQ    72(R14), BX
	SBBQ    80(R14), SI
	SBBQ    88(R14), DI
	MOVQ    $0xb9feffffffffaaab, R8
	MOVQ    $0x1eabfffeb153ffff, R9
	MOVQ    $0x6730d2a0f6b0f624, R10
	MOVQ    $0x64774b84f38512bf, R11
	MOVQ    $0x4b1ba7b6434bacd7, R12
	MOVQ    $0x1a0111ea397fe69a, R13
	CMOVQCC R15, R8
	CMOVQCC R15, R9
	CMOVQCC R15, R10
	CMOVQCC R15, R11
	CMOVQCC R15, R12
	CMOVQCC R15, R13
	ADDQ    R8, AX
	ADCQ    R9, DX
	ADCQ    R10, CX
	ADCQ    R11, BX
	ADCQ    R12, SI
	ADCQ    R13, DI
	MOVQ    48(R14), R8
	MOVQ    56(R14), R9
	MOVQ    64(R14), R10
	MOVQ    72(R14), R11
	MOVQ    80(R14), R12
	MOVQ    88(R14), R13
	ADDQ    0(R14), R8
	ADCQ    8(R14), R9
	ADCQ    16(R14), R10
	ADCQ    24(R14), R11
	ADCQ    32(R14), R12
	ADCQ    40(R14), R13
	MOVQ    res+0(FP), R14
	MOVQ    AX, 0(R14)
	MOVQ    DX, 8(R14)
	MOVQ    CX, 16(R14)
	MOVQ    BX, 24(R14)
	MOVQ    SI, 32(R14)
	MOVQ    DI, 40(R14)

	// reduce element(R8,R9,R10,R11,R12,R13,) using temp registers (AX,DX,CX,BX,SI,DI,R15)
	REDUCE_NOGLOBAL(R8,R9,R10,R11,R12,R13,AX,DX,CX,BX,SI,DI,R15)

	MOVQ R8, 48(R14)
	MOVQ R9, 56(R14)
	MOVQ R10, 64(R14)
	MOVQ R11, 72(R14)
	MOVQ R12, 80(R14)
	MOVQ R13, 88(R14)
	RET

TEXT ·mulAdxE2(SB), $144-24
	NO_LOCAL_POINTERS
	CMPB ·supportAdx(SB), $1
	JNE  l4

	// var a, b, c fp.Element
	// a.Add(&x.A0, &x.A1)
	// b.Add(&y.A0, &y.A1)
	// a.Mul(&a, &b)
	// b.Mul(&x.A0, &y.A0)
	// c.Mul(&x.A1, &y.A1)
	// z.A1.Sub(&a, &b).Sub(&z.A1, &c)
	// z.A0.Sub(&b, &c)

	MOVQ $const_q0, AX
	MOVQ AX, s0-8(SP)
	MOVQ $const_q1, AX
	MOVQ AX, s1-16(SP)
	MOVQ $const_q2, AX
	MOVQ AX, s2-24(SP)
	MOVQ $const_q3, AX
	MOVQ AX, s3-32(SP)
	MOVQ $const_q4, AX
	MOVQ AX, s4-40(SP)
	MOVQ $const_q5, AX
	MOVQ AX, s5-48(SP)
	MOVQ x+8(FP), AX
	MOVQ 48(AX), R13
	MOVQ 56(AX), R14
	MOVQ 64(AX), CX
	MOVQ 72(AX), BX
	MOVQ 80(AX), SI
	MOVQ 88(AX), DI

	// A -> BP
	// t[0] -> R8
	// t[1] -> R9
	// t[2] -> R10
	// t[3] -> R11
	// t[4] -> R12
	// t[5] -> R15
#define MACC(in0, in1, in2) \
	ADCXQ in0, in1     \
	MULXQ in2, AX, in0 \
	ADOXQ AX, in1      \

#define DIV_SHIFT() \
	PUSHQ BP                  \
	MOVQ  $const_qInvNeg, DX  \
	IMULQ R8, DX              \
	XORQ  AX, AX              \
	MULXQ s0-8(SP), AX, BP    \
	ADCXQ R8, AX              \
	MOVQ  BP, R8              \
	POPQ  BP                  \
	MACC(R9, R8, s1-16(SP))   \
	MACC(R10, R9, s2-24(SP))  \
	MACC(R11, R10, s3-32(SP)) \
	MACC(R12, R11, s4-40(SP)) \
	MACC(R15, R12, s5-48(SP)) \
	MOVQ  $0, AX              \
	ADCXQ AX, R15             \
	ADOXQ BP, R15             \

#define MUL_WORD_0() \
	XORQ  AX, AX       \
	MULXQ R13, R8, R9  \
	MULXQ R14, AX, R10 \
	ADOXQ AX, R9       \
	MULXQ CX, AX, R11  \
	ADOXQ AX, R10      \
	MULXQ BX, AX, R12  \
	ADOXQ AX, R11      \
	MULXQ SI, AX, R15  \
	ADOXQ AX, R12      \
	MULXQ DI, AX, BP   \
	ADOXQ AX, R15      \
	MOVQ  $0, AX       \
	ADOXQ AX, BP       \
	DIV_SHIFT()        \

#define MUL_WORD_N() \
	XORQ  AX, AX      \
	MULXQ R13, AX, BP \
	ADOXQ AX, R8      \
	MACC(BP, R9, R14) \
	MACC(BP, R10, CX) \
	MACC(BP, R11, BX) \
	MACC(BP, R12, SI) \
	MACC(BP, R15, DI) \
	MOVQ  $0, AX      \
	ADCXQ AX, BP      \
	ADOXQ AX, BP      \
	DIV_SHIFT()       \

	// mul body
	MOVQ y+16(FP), DX
	MOVQ 48(DX), DX
	MUL_WORD_0()
	MOVQ y+16(FP), DX
	MOVQ 56(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 64(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 72(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 80(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 88(DX), DX
	MUL_WORD_N()

	// reduce element(R8,R9,R10,R11,R12,R15) using temp registers (R13,R14,CX,BX,SI,DI)
	REDUCE(R8,R9,R10,R11,R12,R15,R13,R14,CX,BX,SI,DI,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP),s5-48(SP))

	MOVQ R8, s12-104(SP)
	MOVQ R9, s13-112(SP)
	MOVQ R10, s14-120(SP)
	MOVQ R11, s15-128(SP)
	MOVQ R12, s16-136(SP)
	MOVQ R15, s17-144(SP)
	MOVQ x+8(FP), AX
	MOVQ y+16(FP), DX
	MOVQ 48(AX), R13
	MOVQ 56(AX), R14
	MOVQ 64(AX), CX
	MOVQ 72(AX), BX
	MOVQ 80(AX), SI
	MOVQ 88(AX), DI
	ADDQ 0(AX), R13
	ADCQ 8(AX), R14
	ADCQ 16(AX), CX
	ADCQ 24(AX), BX
	ADCQ 32(AX), SI
	ADCQ 40(AX), DI
	MOVQ R13, s6-56(SP)
	MOVQ R14, s7-64(SP)
	MOVQ CX, s8-72(SP)
	MOVQ BX, s9-80(SP)
	MOVQ SI, s10-88(SP)
	MOVQ DI, s11-96(SP)
	MOVQ 0(DX), R13
	MOVQ 8(DX), R14
	MOVQ 16(DX), CX
	MOVQ 24(DX), BX
	MOVQ 32(DX), SI
	MOVQ 40(DX), DI
	ADDQ 48(DX), R13
	ADCQ 56(DX), R14
	ADCQ 64(DX), CX
	ADCQ 72(DX), BX
	ADCQ 80(DX), SI
	ADCQ 88(DX), DI

	// A -> BP
	// t[0] -> R8
	// t[1] -> R9
	// t[2] -> R10
	// t[3] -> R11
	// t[4] -> R12
	// t[5] -> R15
	// mul body
	MOVQ s6-56(SP), DX
	MUL_WORD_0()
	MOVQ s7-64(SP), DX
	MUL_WORD_N()
	MOVQ s8-72(SP), DX
	MUL_WORD_N()
	MOVQ s9-80(SP), DX
	MUL_WORD_N()
	MOVQ s10-88(SP), DX
	MUL_WORD_N()
	MOVQ s11-96(SP), DX
	MUL_WORD_N()

	// reduce element(R8,R9,R10,R11,R12,R15) using temp registers (R13,R14,CX,BX,SI,DI)
	REDUCE(R8,R9,R10,R11,R12,R15,R13,R14,CX,BX,SI,DI,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP),s5-48(SP))

	MOVQ R8, s6-56(SP)
	MOVQ R9, s7-64(SP)
	MOVQ R10, s8-72(SP)
	MOVQ R11, s9-80(SP)
	MOVQ R12, s10-88(SP)
	MOVQ R15, s11-96(SP)
	MOVQ x+8(FP), AX
	MOVQ 0(AX), R13
	MOVQ 8(AX), R14
	MOVQ 16(AX), CX
	MOVQ 24(AX), BX
	MOVQ 32(AX), SI
	MOVQ 40(AX), DI

	// A -> BP
	// t[0] -> R8
	// t[1] -> R9
	// t[2] -> R10
	// t[3] -> R11
	// t[4] -> R12
	// t[5] -> R15
	// mul body
	MOVQ y+16(FP), DX
	MOVQ 0(DX), DX
	MUL_WORD_0()
	MOVQ y+16(FP), DX
	MOVQ 8(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 16(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 24(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 32(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 40(DX), DX
	MUL_WORD_N()

	// reduce element(R8,R9,R10,R11,R12,R15) using temp registers (R13,R14,CX,BX,SI,DI)
	REDUCE(R8,R9,R10,R11,R12,R15,R13,R14,CX,BX,SI,DI,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP),s5-48(SP))

	XORQ    DX, DX
	MOVQ    s6-56(SP), R13
	MOVQ    s7-64(SP), R14
	MOVQ    s8-72(SP), CX
	MOVQ    s9-80(SP), BX
	MOVQ    s10-88(SP), SI
	MOVQ    s11-96(SP), DI
	SUBQ    R8, R13
	SBBQ    R9, R14
	SBBQ    R10, CX
	SBBQ    R11, BX
	SBBQ    R12, SI
	SBBQ    R15, DI
	MOVQ    R8, s6-56(SP)
	MOVQ    R9, s7-64(SP)
	MOVQ    R10, s8-72(SP)
	MOVQ    R11, s9-80(SP)
	MOVQ    R12, s10-88(SP)
	MOVQ    R15, s11-96(SP)
	MOVQ    $0xb9feffffffffaaab, R8
	MOVQ    $0x1eabfffeb153ffff, R9
	MOVQ    $0x6730d2a0f6b0f624, R10
	MOVQ    $0x64774b84f38512bf, R11
	MOVQ    $0x4b1ba7b6434bacd7, R12
	MOVQ    $0x1a0111ea397fe69a, R15
	CMOVQCC DX, R8
	CMOVQCC DX, R9
	CMOVQCC DX, R10
	CMOVQCC DX, R11
	CMOVQCC DX, R12
	CMOVQCC DX, R15
	ADDQ    R8, R13
	ADCQ    R9, R14
	ADCQ    R10, CX
	ADCQ    R11, BX
	ADCQ    R12, SI
	ADCQ    R15, DI
	SUBQ    s12-104(SP), R13
	SBBQ    s13-112(SP), R14
	SBBQ    s14-120(SP), CX
	SBBQ    s15-128(SP), BX
	SBBQ    s16-136(SP), SI
	SBBQ    s17-144(SP), DI
	MOVQ    $0xb9feffffffffaaab, R8
	MOVQ    $0x1eabfffeb153ffff, R9
	MOVQ    $0x6730d2a0f6b0f624, R10
	MOVQ    $0x64774b84f38512bf, R11
	MOVQ    $0x4b1ba7b6434bacd7, R12
	MOVQ    $0x1a0111ea397fe69a, R15
	CMOVQCC DX, R8
	CMOVQCC DX, R9
	CMOVQCC DX, R10
	CMOVQCC DX, R11
	CMOVQCC DX, R12
	CMOVQCC DX, R15
	ADDQ    R8, R13
	ADCQ    R9, R14
	ADCQ    R10, CX
	ADCQ    R11, BX
	ADCQ    R12, SI
	ADCQ    R15, DI
	MOVQ    res+0(FP), AX
	MOVQ    R13, 48(AX)
	MOVQ    R14, 56(AX)
	MOVQ    CX, 64(AX)
	MOVQ    BX, 72(AX)
	MOVQ    SI, 80(AX)
	MOVQ    DI, 88(AX)
	MOVQ    s6-56(SP), R8
	MOVQ    s7-64(SP), R9
	MOVQ    s8-72(SP), R10
	MOVQ    s9-80(SP), R11
	MOVQ    s10-88(SP), R12
	MOVQ    s11-96(SP), R15
	SUBQ    s12-104(SP), R8
	SBBQ    s13-112(SP), R9
	SBBQ    s14-120(SP), R10
	SBBQ    s15-128(SP), R11
	SBBQ    s16-136(SP), R12
	SBBQ    s17-144(SP), R15
	MOVQ    $0xb9feffffffffaaab, R13
	MOVQ    $0x1eabfffeb153ffff, R14
	MOVQ    $0x6730d2a0f6b0f624, CX
	MOVQ    $0x64774b84f38512bf, BX
	MOVQ    $0x4b1ba7b6434bacd7, SI
	MOVQ    $0x1a0111ea397fe69a, DI
	CMOVQCC DX, R13
	CMOVQCC DX, R14
	CMOVQCC DX, CX
	CMOVQCC DX, BX
	CMOVQCC DX, SI
	CMOVQCC DX, DI
	ADDQ    R13, R8
	ADCQ    R14, R9
	ADCQ    CX, R10
	ADCQ    BX, R11
	ADCQ    SI, R12
	ADCQ    DI, R15
	MOVQ    R8, 0(AX)
	MOVQ    R9, 8(AX)
	MOVQ    R10, 16(AX)
	MOVQ    R11, 24(AX)
	MOVQ    R12, 32(AX)
	MOVQ    R15, 40(AX)
	RET

l4:
	MOVQ res+0(FP), AX
	MOVQ AX, (SP)
	MOVQ x+8(FP), AX
	MOVQ AX, 8(SP)
	MOVQ y+16(FP), AX
	MOVQ AX, 16(SP)
	CALL ·mulGenericE2(SB)
	RET

TEXT ·squareAdxE2(SB), $96-16
	NO_LOCAL_POINTERS
	CMPB ·supportAdx(SB), $1
	JNE  l5

	// z.A0 = (x.A0 + x.A1) * (x.A0 - x.A1)
	// z.A1 = 2 * x.A0 * x.A1

	MOVQ $const_q0, AX
	MOVQ AX, s0-8(SP)
	MOVQ $const_q1, AX
	MOVQ AX, s1-16(SP)
	MOVQ $const_q2, AX
	MOVQ AX, s2-24(SP)
	MOVQ $const_q3, AX
	MOVQ AX, s3-32(SP)
	MOVQ $const_q4, AX
	MOVQ AX, s4-40(SP)
	MOVQ $const_q5, AX
	MOVQ AX, s5-48(SP)

	// 2 * x.A0 * x.A1
	MOVQ x+8(FP), AX

	// 2 * x.A1[0] -> R13
	// 2 * x.A1[1] -> R14
	// 2 * x.A1[2] -> CX
	// 2 * x.A1[3] -> BX
	// 2 * x.A1[4] -> SI
	// 2 * x.A1[5] -> DI
	MOVQ 48(AX), R13
	MOVQ 56(AX), R14
	MOVQ 64(AX), CX
	MOVQ 72(AX), BX
	MOVQ 80(AX), SI
	MOVQ 88(AX), DI
	ADDQ R13, R13
	ADCQ R14, R14
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI

	// A -> BP
	// t[0] -> R8
	// t[1] -> R9
	// t[2] -> R10
	// t[3] -> R11
	// t[4] -> R12
	// t[5] -> R15
	// mul body
	MOVQ x+8(FP), DX
	MOVQ 0(DX), DX
	MUL_WORD_0()
	MOVQ x+8(FP), DX
	MOVQ 8(DX), DX
	MUL_WORD_N()
	MOVQ x+8(FP), DX
	MOVQ 16(DX), DX
	MUL_WORD_N()
	MOVQ x+8(FP), DX
	MOVQ 24(DX), DX
	MUL_WORD_N()
	MOVQ x+8(FP), DX
	MOVQ 32(DX), DX
	MUL_WORD_N()
	MOVQ x+8(FP), DX
	MOVQ 40(DX), DX
	MUL_WORD_N()

	// reduce element(R8,R9,R10,R11,R12,R15) using temp registers (R13,R14,CX,BX,SI,DI)
	REDUCE(R8,R9,R10,R11,R12,R15,R13,R14,CX,BX,SI,DI,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP),s5-48(SP))

	MOVQ x+8(FP), AX

	// x.A1[0] -> R13
	// x.A1[1] -> R14
	// x.A1[2] -> CX
	// x.A1[3] -> BX
	// x.A1[4] -> SI
	// x.A1[5] -> DI
	MOVQ 48(AX), R13
	MOVQ 56(AX), R14
	MOVQ 64(AX), CX
	MOVQ 72(AX), BX
	MOVQ 80(AX), SI
	MOVQ 88(AX), DI
	MOVQ res+0(FP), DX
	MOVQ R8, 48(DX)
	MOVQ R9, 56(DX)
	MOVQ R10, 64(DX)
	MOVQ R11, 72(DX)
	MOVQ R12, 80(DX)
	MOVQ R15, 88(DX)
	MOVQ R13, R8
	MOVQ R14, R9
	MOVQ CX, R10
	MOVQ BX, R11
	MOVQ SI, R12
	MOVQ DI, R15

	// Add(&x.A0, &x.A1)
	ADDQ 0(AX), R13
	ADCQ 8(AX), R14
	ADCQ 16(AX), CX
	ADCQ 24(AX), BX
	ADCQ 32(AX), SI
	ADCQ 40(AX), DI
	MOVQ R13, s6-56(SP)
	MOVQ R14, s7-64(SP)
	MOVQ CX, s8-72(SP)
	MOVQ BX, s9-80(SP)
	MOVQ SI, s10-88(SP)
	MOVQ DI, s11-96(SP)
	XORQ DX, DX

	// Sub(&x.A0, &x.A1)
	MOVQ    0(AX), R13
	MOVQ    8(AX), R14
	MOVQ    16(AX), CX
	MOVQ    24(AX), BX
	MOVQ    32(AX), SI
	MOVQ    40(AX), DI
	SUBQ    R8, R13
	SBBQ    R9, R14
	SBBQ    R10, CX
	SBBQ    R11, BX
	SBBQ    R12, SI
	SBBQ    R15, DI
	MOVQ    $0xb9feffffffffaaab, R8
	MOVQ    $0x1eabfffeb153ffff, R9
	MOVQ    $0x6730d2a0f6b0f624, R10
	MOVQ    $0x64774b84f38512bf, R11
	MOVQ    $0x4b1ba7b6434bacd7, R12
	MOVQ    $0x1a0111ea397fe69a, R15
	CMOVQCC DX, R8
	CMOVQCC DX, R9
	CMOVQCC DX, R10
	CMOVQCC DX, R11
	CMOVQCC DX, R12
	CMOVQCC DX, R15
	ADDQ    R8, R13
	ADCQ    R9, R14
	ADCQ    R10, CX
	ADCQ    R11, BX
	ADCQ    R12, SI
	ADCQ    R15, DI

	// A -> BP
	// t[0] -> R8
	// t[1] -> R9
	// t[2] -> R10
	// t[3] -> R11
	// t[4] -> R12
	// t[5] -> R15
	// mul body
	MOVQ s6-56(SP), DX
	MUL_WORD_0()
	MOVQ s7-64(SP), DX
	MUL_WORD_N()
	MOVQ s8-72(SP), DX
	MUL_WORD_N()
	MOVQ s9-80(SP), DX
	MUL_WORD_N()
	MOVQ s10-88(SP), DX
	MUL_WORD_N()
	MOVQ s11-96(SP), DX
	MUL_WORD_N()

	// reduce element(R8,R9,R10,R11,R12,R15) using temp registers (R13,R14,CX,BX,SI,DI)
	REDUCE(R8,R9,R10,R11,R12,R15,R13,R14,CX,BX,SI,DI,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP),s5-48(SP))

	MOVQ res+0(FP), AX
	MOVQ R8, 0(AX)
	MOVQ R9, 8(AX)
	MOVQ R10, 16(AX)
	MOVQ R11, 24(AX)
	MOVQ R12, 32(AX)
	MOVQ R15, 40(AX)
	RET

l5:
	MOVQ res+0(FP), AX
	MOVQ AX, (SP)
	MOVQ x+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·squareGenericE2(SB)
	RET
