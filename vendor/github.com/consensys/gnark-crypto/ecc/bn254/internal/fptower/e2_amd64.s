// Copyright 2020-2025 Consensys Software Inc.
// Licensed under the Apache License, Version 2.0. See the LICENSE file for details.

#include "textflag.h"
#include "funcdata.h"
#include "go_asm.h"

#define REDUCE(ra0, ra1, ra2, ra3, rb0, rb1, rb2, rb3, q0, q1, q2, q3) \
	MOVQ    ra0, rb0; \
	SUBQ    q0, ra0;  \
	MOVQ    ra1, rb1; \
	SBBQ    q1, ra1;  \
	MOVQ    ra2, rb2; \
	SBBQ    q2, ra2;  \
	MOVQ    ra3, rb3; \
	SBBQ    q3, ra3;  \
	CMOVQCS rb0, ra0; \
	CMOVQCS rb1, ra1; \
	CMOVQCS rb2, ra2; \
	CMOVQCS rb3, ra3; \

#define REDUCE_NOGLOBAL(ra0, ra1, ra2, ra3, rb0, rb1, rb2, rb3, scratch0) \
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
	CMOVQCS rb0, ra0;            \
	CMOVQCS rb1, ra1;            \
	CMOVQCS rb2, ra2;            \
	CMOVQCS rb3, ra3;            \

TEXT ·addE2(SB), NOSPLIT, $0-24
	MOVQ x+8(FP), AX
	MOVQ 0(AX), CX
	MOVQ 8(AX), BX
	MOVQ 16(AX), SI
	MOVQ 24(AX), DI
	MOVQ y+16(FP), DX
	ADDQ 0(DX), CX
	ADCQ 8(DX), BX
	ADCQ 16(DX), SI
	ADCQ 24(DX), DI

	// reduce element(CX,BX,SI,DI,) using temp registers (R9,R10,R11,R12,R8)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R9,R10,R11,R12,R8)

	MOVQ res+0(FP), R13
	MOVQ CX, 0(R13)
	MOVQ BX, 8(R13)
	MOVQ SI, 16(R13)
	MOVQ DI, 24(R13)
	MOVQ 32(AX), CX
	MOVQ 40(AX), BX
	MOVQ 48(AX), SI
	MOVQ 56(AX), DI
	ADDQ 32(DX), CX
	ADCQ 40(DX), BX
	ADCQ 48(DX), SI
	ADCQ 56(DX), DI

	// reduce element(CX,BX,SI,DI,) using temp registers (R15,R9,R10,R11,R14)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R15,R9,R10,R11,R14)

	MOVQ CX, 32(R13)
	MOVQ BX, 40(R13)
	MOVQ SI, 48(R13)
	MOVQ DI, 56(R13)
	RET

TEXT ·doubleE2(SB), NOSPLIT, $0-16
	MOVQ res+0(FP), DX
	MOVQ x+8(FP), AX
	MOVQ 0(AX), CX
	MOVQ 8(AX), BX
	MOVQ 16(AX), SI
	MOVQ 24(AX), DI
	ADDQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI

	// reduce element(CX,BX,SI,DI,) using temp registers (R9,R10,R11,R12,R8)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R9,R10,R11,R12,R8)

	MOVQ CX, 0(DX)
	MOVQ BX, 8(DX)
	MOVQ SI, 16(DX)
	MOVQ DI, 24(DX)
	MOVQ 32(AX), CX
	MOVQ 40(AX), BX
	MOVQ 48(AX), SI
	MOVQ 56(AX), DI
	ADDQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI

	// reduce element(CX,BX,SI,DI,) using temp registers (R14,R15,R9,R10,R13)
	REDUCE_NOGLOBAL(CX,BX,SI,DI,R14,R15,R9,R10,R13)

	MOVQ CX, 32(DX)
	MOVQ BX, 40(DX)
	MOVQ SI, 48(DX)
	MOVQ DI, 56(DX)
	RET

TEXT ·subE2(SB), NOSPLIT, $0-24
	XORQ    R15, R15
	MOVQ    x+8(FP), SI
	MOVQ    0(SI), AX
	MOVQ    8(SI), DX
	MOVQ    16(SI), CX
	MOVQ    24(SI), BX
	MOVQ    y+16(FP), SI
	SUBQ    0(SI), AX
	SBBQ    8(SI), DX
	SBBQ    16(SI), CX
	SBBQ    24(SI), BX
	MOVQ    x+8(FP), SI
	MOVQ    $0x3c208c16d87cfd47, DI
	MOVQ    $0x97816a916871ca8d, R8
	MOVQ    $0xb85045b68181585d, R9
	MOVQ    $0x30644e72e131a029, R10
	CMOVQCC R15, DI
	CMOVQCC R15, R8
	CMOVQCC R15, R9
	CMOVQCC R15, R10
	ADDQ    DI, AX
	ADCQ    R8, DX
	ADCQ    R9, CX
	ADCQ    R10, BX
	MOVQ    res+0(FP), R11
	MOVQ    AX, 0(R11)
	MOVQ    DX, 8(R11)
	MOVQ    CX, 16(R11)
	MOVQ    BX, 24(R11)
	MOVQ    32(SI), AX
	MOVQ    40(SI), DX
	MOVQ    48(SI), CX
	MOVQ    56(SI), BX
	MOVQ    y+16(FP), SI
	SUBQ    32(SI), AX
	SBBQ    40(SI), DX
	SBBQ    48(SI), CX
	SBBQ    56(SI), BX
	MOVQ    $0x3c208c16d87cfd47, R12
	MOVQ    $0x97816a916871ca8d, R13
	MOVQ    $0xb85045b68181585d, R14
	MOVQ    $0x30644e72e131a029, DI
	CMOVQCC R15, R12
	CMOVQCC R15, R13
	CMOVQCC R15, R14
	CMOVQCC R15, DI
	ADDQ    R12, AX
	ADCQ    R13, DX
	ADCQ    R14, CX
	ADCQ    DI, BX
	MOVQ    res+0(FP), SI
	MOVQ    AX, 32(SI)
	MOVQ    DX, 40(SI)
	MOVQ    CX, 48(SI)
	MOVQ    BX, 56(SI)
	RET

TEXT ·negE2(SB), NOSPLIT, $0-16
	MOVQ  res+0(FP), DX
	MOVQ  x+8(FP), AX
	MOVQ  0(AX), BX
	MOVQ  8(AX), SI
	MOVQ  16(AX), DI
	MOVQ  24(AX), R8
	MOVQ  BX, AX
	ORQ   SI, AX
	ORQ   DI, AX
	ORQ   R8, AX
	TESTQ AX, AX
	JNE   l1
	MOVQ  AX, 0(DX)
	MOVQ  AX, 8(DX)
	MOVQ  AX, 16(DX)
	MOVQ  AX, 24(DX)
	JMP   l3

l1:
	MOVQ $0x3c208c16d87cfd47, CX
	SUBQ BX, CX
	MOVQ CX, 0(DX)
	MOVQ $0x97816a916871ca8d, CX
	SBBQ SI, CX
	MOVQ CX, 8(DX)
	MOVQ $0xb85045b68181585d, CX
	SBBQ DI, CX
	MOVQ CX, 16(DX)
	MOVQ $0x30644e72e131a029, CX
	SBBQ R8, CX
	MOVQ CX, 24(DX)

l3:
	MOVQ  x+8(FP), AX
	MOVQ  32(AX), BX
	MOVQ  40(AX), SI
	MOVQ  48(AX), DI
	MOVQ  56(AX), R8
	MOVQ  BX, AX
	ORQ   SI, AX
	ORQ   DI, AX
	ORQ   R8, AX
	TESTQ AX, AX
	JNE   l2
	MOVQ  AX, 32(DX)
	MOVQ  AX, 40(DX)
	MOVQ  AX, 48(DX)
	MOVQ  AX, 56(DX)
	RET

l2:
	MOVQ $0x3c208c16d87cfd47, CX
	SUBQ BX, CX
	MOVQ CX, 32(DX)
	MOVQ $0x97816a916871ca8d, CX
	SBBQ SI, CX
	MOVQ CX, 40(DX)
	MOVQ $0xb85045b68181585d, CX
	SBBQ DI, CX
	MOVQ CX, 48(DX)
	MOVQ $0x30644e72e131a029, CX
	SBBQ R8, CX
	MOVQ CX, 56(DX)
	RET

TEXT ·mulNonResE2(SB), NOSPLIT, $0-16
	MOVQ x+8(FP), R10
	MOVQ 0(R10), AX
	MOVQ 8(R10), DX
	MOVQ 16(R10), CX
	MOVQ 24(R10), BX
	ADDQ AX, AX
	ADCQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX

	// reduce element(AX,DX,CX,BX) using temp registers (R11,R12,R13,R14)
	REDUCE(AX,DX,CX,BX,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ AX, AX
	ADCQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX

	// reduce element(AX,DX,CX,BX) using temp registers (R11,R12,R13,R14)
	REDUCE(AX,DX,CX,BX,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ AX, AX
	ADCQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX

	// reduce element(AX,DX,CX,BX) using temp registers (R11,R12,R13,R14)
	REDUCE(AX,DX,CX,BX,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ 0(R10), AX
	ADCQ 8(R10), DX
	ADCQ 16(R10), CX
	ADCQ 24(R10), BX

	// reduce element(AX,DX,CX,BX) using temp registers (R11,R12,R13,R14)
	REDUCE(AX,DX,CX,BX,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	MOVQ    32(R10), SI
	MOVQ    40(R10), DI
	MOVQ    48(R10), R8
	MOVQ    56(R10), R9
	XORQ    R15, R15
	SUBQ    SI, AX
	SBBQ    DI, DX
	SBBQ    R8, CX
	SBBQ    R9, BX
	MOVQ    $0x3c208c16d87cfd47, R11
	MOVQ    $0x97816a916871ca8d, R12
	MOVQ    $0xb85045b68181585d, R13
	MOVQ    $0x30644e72e131a029, R14
	CMOVQCC R15, R11
	CMOVQCC R15, R12
	CMOVQCC R15, R13
	CMOVQCC R15, R14
	ADDQ    R11, AX
	ADCQ    R12, DX
	ADCQ    R13, CX
	ADCQ    R14, BX
	ADDQ    SI, SI
	ADCQ    DI, DI
	ADCQ    R8, R8
	ADCQ    R9, R9

	// reduce element(SI,DI,R8,R9) using temp registers (R11,R12,R13,R14)
	REDUCE(SI,DI,R8,R9,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8
	ADCQ R9, R9

	// reduce element(SI,DI,R8,R9) using temp registers (R11,R12,R13,R14)
	REDUCE(SI,DI,R8,R9,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8
	ADCQ R9, R9

	// reduce element(SI,DI,R8,R9) using temp registers (R11,R12,R13,R14)
	REDUCE(SI,DI,R8,R9,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ 32(R10), SI
	ADCQ 40(R10), DI
	ADCQ 48(R10), R8
	ADCQ 56(R10), R9

	// reduce element(SI,DI,R8,R9) using temp registers (R11,R12,R13,R14)
	REDUCE(SI,DI,R8,R9,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	ADDQ 0(R10), SI
	ADCQ 8(R10), DI
	ADCQ 16(R10), R8
	ADCQ 24(R10), R9

	// reduce element(SI,DI,R8,R9) using temp registers (R11,R12,R13,R14)
	REDUCE(SI,DI,R8,R9,R11,R12,R13,R14,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	MOVQ res+0(FP), R10
	MOVQ AX, 0(R10)
	MOVQ DX, 8(R10)
	MOVQ CX, 16(R10)
	MOVQ BX, 24(R10)
	MOVQ SI, 32(R10)
	MOVQ DI, 40(R10)
	MOVQ R8, 48(R10)
	MOVQ R9, 56(R10)
	RET

TEXT ·mulAdxE2(SB), $48-24
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

	MOVQ x+8(FP), AX
	MOVQ 32(AX), R13
	MOVQ 40(AX), R14
	MOVQ 48(AX), CX
	MOVQ 56(AX), BX

	// A -> BP
	// t[0] -> SI
	// t[1] -> DI
	// t[2] -> R8
	// t[3] -> R9
#define MACC(in0, in1, in2) \
	ADCXQ in0, in1     \
	MULXQ in2, AX, in0 \
	ADOXQ AX, in1      \

#define DIV_SHIFT() \
	MOVQ  $const_qInvNeg, DX       \
	IMULQ SI, DX                   \
	XORQ  AX, AX                   \
	MULXQ ·qElement+0(SB), AX, R10 \
	ADCXQ SI, AX                   \
	MOVQ  R10, SI                  \
	MACC(DI, SI, ·qElement+8(SB))  \
	MACC(R8, DI, ·qElement+16(SB)) \
	MACC(R9, R8, ·qElement+24(SB)) \
	MOVQ  $0, AX                   \
	ADCXQ AX, R9                   \
	ADOXQ BP, R9                   \

#define MUL_WORD_0() \
	XORQ  AX, AX      \
	MULXQ R13, SI, DI \
	MULXQ R14, AX, R8 \
	ADOXQ AX, DI      \
	MULXQ CX, AX, R9  \
	ADOXQ AX, R8      \
	MULXQ BX, AX, BP  \
	ADOXQ AX, R9      \
	MOVQ  $0, AX      \
	ADOXQ AX, BP      \
	DIV_SHIFT()       \

#define MUL_WORD_N() \
	XORQ  AX, AX      \
	MULXQ R13, AX, BP \
	ADOXQ AX, SI      \
	MACC(BP, DI, R14) \
	MACC(BP, R8, CX)  \
	MACC(BP, R9, BX)  \
	MOVQ  $0, AX      \
	ADCXQ AX, BP      \
	ADOXQ AX, BP      \
	DIV_SHIFT()       \

	// mul body
	MOVQ y+16(FP), DX
	MOVQ 32(DX), DX
	MUL_WORD_0()
	MOVQ y+16(FP), DX
	MOVQ 40(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 48(DX), DX
	MUL_WORD_N()
	MOVQ y+16(FP), DX
	MOVQ 56(DX), DX
	MUL_WORD_N()

	// reduce element(SI,DI,R8,R9) using temp registers (R13,R14,CX,BX)
	REDUCE(SI,DI,R8,R9,R13,R14,CX,BX,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	MOVQ SI, s2-24(SP)
	MOVQ DI, s3-32(SP)
	MOVQ R8, s4-40(SP)
	MOVQ R9, s5-48(SP)
	MOVQ x+8(FP), AX
	MOVQ y+16(FP), DX
	MOVQ 32(AX), R13
	MOVQ 40(AX), R14
	MOVQ 48(AX), CX
	MOVQ 56(AX), BX
	ADDQ 0(AX), R13
	ADCQ 8(AX), R14
	ADCQ 16(AX), CX
	ADCQ 24(AX), BX
	MOVQ R13, R11
	MOVQ R14, R12
	MOVQ CX, s0-8(SP)
	MOVQ BX, s1-16(SP)
	MOVQ 0(DX), R13
	MOVQ 8(DX), R14
	MOVQ 16(DX), CX
	MOVQ 24(DX), BX
	ADDQ 32(DX), R13
	ADCQ 40(DX), R14
	ADCQ 48(DX), CX
	ADCQ 56(DX), BX

	// A -> BP
	// t[0] -> SI
	// t[1] -> DI
	// t[2] -> R8
	// t[3] -> R9
	// mul body
	MOVQ R11, DX
	MUL_WORD_0()
	MOVQ R12, DX
	MUL_WORD_N()
	MOVQ s0-8(SP), DX
	MUL_WORD_N()
	MOVQ s1-16(SP), DX
	MUL_WORD_N()

	// reduce element(SI,DI,R8,R9) using temp registers (R13,R14,CX,BX)
	REDUCE(SI,DI,R8,R9,R13,R14,CX,BX,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	MOVQ SI, R11
	MOVQ DI, R12
	MOVQ R8, s0-8(SP)
	MOVQ R9, s1-16(SP)
	MOVQ x+8(FP), AX
	MOVQ 0(AX), R13
	MOVQ 8(AX), R14
	MOVQ 16(AX), CX
	MOVQ 24(AX), BX

	// A -> BP
	// t[0] -> SI
	// t[1] -> DI
	// t[2] -> R8
	// t[3] -> R9
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

	// reduce element(SI,DI,R8,R9) using temp registers (R13,R14,CX,BX)
	REDUCE(SI,DI,R8,R9,R13,R14,CX,BX,·qElement+0(SB),·qElement+8(SB),·qElement+16(SB),·qElement+24(SB))

	XORQ    DX, DX
	MOVQ    R11, R13
	MOVQ    R12, R14
	MOVQ    s0-8(SP), CX
	MOVQ    s1-16(SP), BX
	SUBQ    SI, R13
	SBBQ    DI, R14
	SBBQ    R8, CX
	SBBQ    R9, BX
	MOVQ    SI, R11
	MOVQ    DI, R12
	MOVQ    R8, s0-8(SP)
	MOVQ    R9, s1-16(SP)
	MOVQ    $0x3c208c16d87cfd47, SI
	MOVQ    $0x97816a916871ca8d, DI
	MOVQ    $0xb85045b68181585d, R8
	MOVQ    $0x30644e72e131a029, R9
	CMOVQCC DX, SI
	CMOVQCC DX, DI
	CMOVQCC DX, R8
	CMOVQCC DX, R9
	ADDQ    SI, R13
	ADCQ    DI, R14
	ADCQ    R8, CX
	ADCQ    R9, BX
	SUBQ    s2-24(SP), R13
	SBBQ    s3-32(SP), R14
	SBBQ    s4-40(SP), CX
	SBBQ    s5-48(SP), BX
	MOVQ    $0x3c208c16d87cfd47, SI
	MOVQ    $0x97816a916871ca8d, DI
	MOVQ    $0xb85045b68181585d, R8
	MOVQ    $0x30644e72e131a029, R9
	CMOVQCC DX, SI
	CMOVQCC DX, DI
	CMOVQCC DX, R8
	CMOVQCC DX, R9
	ADDQ    SI, R13
	ADCQ    DI, R14
	ADCQ    R8, CX
	ADCQ    R9, BX
	MOVQ    res+0(FP), AX
	MOVQ    R13, 32(AX)
	MOVQ    R14, 40(AX)
	MOVQ    CX, 48(AX)
	MOVQ    BX, 56(AX)
	MOVQ    R11, SI
	MOVQ    R12, DI
	MOVQ    s0-8(SP), R8
	MOVQ    s1-16(SP), R9
	SUBQ    s2-24(SP), SI
	SBBQ    s3-32(SP), DI
	SBBQ    s4-40(SP), R8
	SBBQ    s5-48(SP), R9
	MOVQ    $0x3c208c16d87cfd47, R13
	MOVQ    $0x97816a916871ca8d, R14
	MOVQ    $0xb85045b68181585d, CX
	MOVQ    $0x30644e72e131a029, BX
	CMOVQCC DX, R13
	CMOVQCC DX, R14
	CMOVQCC DX, CX
	CMOVQCC DX, BX
	ADDQ    R13, SI
	ADCQ    R14, DI
	ADCQ    CX, R8
	ADCQ    BX, R9
	MOVQ    SI, 0(AX)
	MOVQ    DI, 8(AX)
	MOVQ    R8, 16(AX)
	MOVQ    R9, 24(AX)
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

TEXT ·squareAdxE2(SB), $64-16
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

	// 2 * x.A0 * x.A1
	MOVQ x+8(FP), AX

	// 2 * x.A1[0] -> R13
	// 2 * x.A1[1] -> R14
	// 2 * x.A1[2] -> CX
	// 2 * x.A1[3] -> BX
	MOVQ 32(AX), R13
	MOVQ 40(AX), R14
	MOVQ 48(AX), CX
	MOVQ 56(AX), BX
	ADDQ R13, R13
	ADCQ R14, R14
	ADCQ CX, CX
	ADCQ BX, BX

	// A -> BP
	// t[0] -> SI
	// t[1] -> DI
	// t[2] -> R8
	// t[3] -> R9
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

	// reduce element(SI,DI,R8,R9) using temp registers (R13,R14,CX,BX)
	REDUCE(SI,DI,R8,R9,R13,R14,CX,BX,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP))

	MOVQ x+8(FP), AX

	// x.A1[0] -> R13
	// x.A1[1] -> R14
	// x.A1[2] -> CX
	// x.A1[3] -> BX
	MOVQ 32(AX), R13
	MOVQ 40(AX), R14
	MOVQ 48(AX), CX
	MOVQ 56(AX), BX
	MOVQ res+0(FP), DX
	MOVQ SI, 32(DX)
	MOVQ DI, 40(DX)
	MOVQ R8, 48(DX)
	MOVQ R9, 56(DX)
	MOVQ R13, SI
	MOVQ R14, DI
	MOVQ CX, R8
	MOVQ BX, R9

	// Add(&x.A0, &x.A1)
	ADDQ 0(AX), R13
	ADCQ 8(AX), R14
	ADCQ 16(AX), CX
	ADCQ 24(AX), BX
	MOVQ R13, s4-40(SP)
	MOVQ R14, s5-48(SP)
	MOVQ CX, s6-56(SP)
	MOVQ BX, s7-64(SP)
	XORQ DX, DX

	// Sub(&x.A0, &x.A1)
	MOVQ    0(AX), R13
	MOVQ    8(AX), R14
	MOVQ    16(AX), CX
	MOVQ    24(AX), BX
	SUBQ    SI, R13
	SBBQ    DI, R14
	SBBQ    R8, CX
	SBBQ    R9, BX
	MOVQ    $0x3c208c16d87cfd47, SI
	MOVQ    $0x97816a916871ca8d, DI
	MOVQ    $0xb85045b68181585d, R8
	MOVQ    $0x30644e72e131a029, R9
	CMOVQCC DX, SI
	CMOVQCC DX, DI
	CMOVQCC DX, R8
	CMOVQCC DX, R9
	ADDQ    SI, R13
	ADCQ    DI, R14
	ADCQ    R8, CX
	ADCQ    R9, BX

	// A -> BP
	// t[0] -> SI
	// t[1] -> DI
	// t[2] -> R8
	// t[3] -> R9
	// mul body
	MOVQ s4-40(SP), DX
	MUL_WORD_0()
	MOVQ s5-48(SP), DX
	MUL_WORD_N()
	MOVQ s6-56(SP), DX
	MUL_WORD_N()
	MOVQ s7-64(SP), DX
	MUL_WORD_N()

	// reduce element(SI,DI,R8,R9) using temp registers (R13,R14,CX,BX)
	REDUCE(SI,DI,R8,R9,R13,R14,CX,BX,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP))

	MOVQ res+0(FP), AX
	MOVQ SI, 0(AX)
	MOVQ DI, 8(AX)
	MOVQ R8, 16(AX)
	MOVQ R9, 24(AX)
	RET

l5:
	MOVQ res+0(FP), AX
	MOVQ AX, (SP)
	MOVQ x+8(FP), AX
	MOVQ AX, 8(SP)
	CALL ·squareGenericE2(SB)
	RET
