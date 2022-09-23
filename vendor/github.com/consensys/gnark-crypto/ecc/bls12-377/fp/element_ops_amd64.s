// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "textflag.h"
#include "funcdata.h"

// modulus q
DATA q<>+0(SB)/8, $0x8508c00000000001
DATA q<>+8(SB)/8, $0x170b5d4430000000
DATA q<>+16(SB)/8, $0x1ef3622fba094800
DATA q<>+24(SB)/8, $0x1a22d9f300f5138f
DATA q<>+32(SB)/8, $0xc63b05c06ca1493b
DATA q<>+40(SB)/8, $0x01ae3a4617c510ea
GLOBL q<>(SB), (RODATA+NOPTR), $48

// qInv0 q'[0]
DATA qInv0<>(SB)/8, $0x8508bfffffffffff
GLOBL qInv0<>(SB), (RODATA+NOPTR), $8

#define REDUCE(ra0, ra1, ra2, ra3, ra4, ra5, rb0, rb1, rb2, rb3, rb4, rb5) \
	MOVQ    ra0, rb0;        \
	SUBQ    q<>(SB), ra0;    \
	MOVQ    ra1, rb1;        \
	SBBQ    q<>+8(SB), ra1;  \
	MOVQ    ra2, rb2;        \
	SBBQ    q<>+16(SB), ra2; \
	MOVQ    ra3, rb3;        \
	SBBQ    q<>+24(SB), ra3; \
	MOVQ    ra4, rb4;        \
	SBBQ    q<>+32(SB), ra4; \
	MOVQ    ra5, rb5;        \
	SBBQ    q<>+40(SB), ra5; \
	CMOVQCS rb0, ra0;        \
	CMOVQCS rb1, ra1;        \
	CMOVQCS rb2, ra2;        \
	CMOVQCS rb3, ra3;        \
	CMOVQCS rb4, ra4;        \
	CMOVQCS rb5, ra5;        \

// add(res, x, y *Element)
TEXT ·add(SB), NOSPLIT, $0-24
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

	// reduce element(CX,BX,SI,DI,R8,R9) using temp registers (R10,R11,R12,R13,R14,R15)
	REDUCE(CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14,R15)

	MOVQ res+0(FP), AX
	MOVQ CX, 0(AX)
	MOVQ BX, 8(AX)
	MOVQ SI, 16(AX)
	MOVQ DI, 24(AX)
	MOVQ R8, 32(AX)
	MOVQ R9, 40(AX)
	RET

// sub(res, x, y *Element)
TEXT ·sub(SB), NOSPLIT, $0-24
	XORQ    R9, R9
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
	MOVQ    $0x8508c00000000001, R10
	MOVQ    $0x170b5d4430000000, R11
	MOVQ    $0x1ef3622fba094800, R12
	MOVQ    $0x1a22d9f300f5138f, R13
	MOVQ    $0xc63b05c06ca1493b, R14
	MOVQ    $0x01ae3a4617c510ea, R15
	CMOVQCC R9, R10
	CMOVQCC R9, R11
	CMOVQCC R9, R12
	CMOVQCC R9, R13
	CMOVQCC R9, R14
	CMOVQCC R9, R15
	ADDQ    R10, AX
	ADCQ    R11, DX
	ADCQ    R12, CX
	ADCQ    R13, BX
	ADCQ    R14, SI
	ADCQ    R15, DI
	MOVQ    res+0(FP), R8
	MOVQ    AX, 0(R8)
	MOVQ    DX, 8(R8)
	MOVQ    CX, 16(R8)
	MOVQ    BX, 24(R8)
	MOVQ    SI, 32(R8)
	MOVQ    DI, 40(R8)
	RET

// double(res, x *Element)
TEXT ·double(SB), NOSPLIT, $0-16
	MOVQ x+8(FP), AX
	MOVQ 0(AX), DX
	MOVQ 8(AX), CX
	MOVQ 16(AX), BX
	MOVQ 24(AX), SI
	MOVQ 32(AX), DI
	MOVQ 40(AX), R8
	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	MOVQ res+0(FP), R15
	MOVQ DX, 0(R15)
	MOVQ CX, 8(R15)
	MOVQ BX, 16(R15)
	MOVQ SI, 24(R15)
	MOVQ DI, 32(R15)
	MOVQ R8, 40(R15)
	RET

// neg(res, x *Element)
TEXT ·neg(SB), NOSPLIT, $0-16
	MOVQ  res+0(FP), R9
	MOVQ  x+8(FP), AX
	MOVQ  0(AX), DX
	MOVQ  8(AX), CX
	MOVQ  16(AX), BX
	MOVQ  24(AX), SI
	MOVQ  32(AX), DI
	MOVQ  40(AX), R8
	MOVQ  DX, AX
	ORQ   CX, AX
	ORQ   BX, AX
	ORQ   SI, AX
	ORQ   DI, AX
	ORQ   R8, AX
	TESTQ AX, AX
	JEQ   l1
	MOVQ  $0x8508c00000000001, R10
	SUBQ  DX, R10
	MOVQ  R10, 0(R9)
	MOVQ  $0x170b5d4430000000, R10
	SBBQ  CX, R10
	MOVQ  R10, 8(R9)
	MOVQ  $0x1ef3622fba094800, R10
	SBBQ  BX, R10
	MOVQ  R10, 16(R9)
	MOVQ  $0x1a22d9f300f5138f, R10
	SBBQ  SI, R10
	MOVQ  R10, 24(R9)
	MOVQ  $0xc63b05c06ca1493b, R10
	SBBQ  DI, R10
	MOVQ  R10, 32(R9)
	MOVQ  $0x01ae3a4617c510ea, R10
	SBBQ  R8, R10
	MOVQ  R10, 40(R9)
	RET

l1:
	MOVQ AX, 0(R9)
	MOVQ AX, 8(R9)
	MOVQ AX, 16(R9)
	MOVQ AX, 24(R9)
	MOVQ AX, 32(R9)
	MOVQ AX, 40(R9)
	RET

TEXT ·reduce(SB), NOSPLIT, $0-8
	MOVQ res+0(FP), AX
	MOVQ 0(AX), DX
	MOVQ 8(AX), CX
	MOVQ 16(AX), BX
	MOVQ 24(AX), SI
	MOVQ 32(AX), DI
	MOVQ 40(AX), R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	MOVQ DX, 0(AX)
	MOVQ CX, 8(AX)
	MOVQ BX, 16(AX)
	MOVQ SI, 24(AX)
	MOVQ DI, 32(AX)
	MOVQ R8, 40(AX)
	RET

// MulBy3(x *Element)
TEXT ·MulBy3(SB), NOSPLIT, $0-8
	MOVQ x+0(FP), AX
	MOVQ 0(AX), DX
	MOVQ 8(AX), CX
	MOVQ 16(AX), BX
	MOVQ 24(AX), SI
	MOVQ 32(AX), DI
	MOVQ 40(AX), R8
	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	ADDQ 0(AX), DX
	ADCQ 8(AX), CX
	ADCQ 16(AX), BX
	ADCQ 24(AX), SI
	ADCQ 32(AX), DI
	ADCQ 40(AX), R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R15,R9,R10,R11,R12,R13)
	REDUCE(DX,CX,BX,SI,DI,R8,R15,R9,R10,R11,R12,R13)

	MOVQ DX, 0(AX)
	MOVQ CX, 8(AX)
	MOVQ BX, 16(AX)
	MOVQ SI, 24(AX)
	MOVQ DI, 32(AX)
	MOVQ R8, 40(AX)
	RET

// MulBy5(x *Element)
TEXT ·MulBy5(SB), NOSPLIT, $0-8
	MOVQ x+0(FP), AX
	MOVQ 0(AX), DX
	MOVQ 8(AX), CX
	MOVQ 16(AX), BX
	MOVQ 24(AX), SI
	MOVQ 32(AX), DI
	MOVQ 40(AX), R8
	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R15,R9,R10,R11,R12,R13)
	REDUCE(DX,CX,BX,SI,DI,R8,R15,R9,R10,R11,R12,R13)

	ADDQ 0(AX), DX
	ADCQ 8(AX), CX
	ADCQ 16(AX), BX
	ADCQ 24(AX), SI
	ADCQ 32(AX), DI
	ADCQ 40(AX), R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R14,R15,R9,R10,R11,R12)
	REDUCE(DX,CX,BX,SI,DI,R8,R14,R15,R9,R10,R11,R12)

	MOVQ DX, 0(AX)
	MOVQ CX, 8(AX)
	MOVQ BX, 16(AX)
	MOVQ SI, 24(AX)
	MOVQ DI, 32(AX)
	MOVQ R8, 40(AX)
	RET

// MulBy13(x *Element)
TEXT ·MulBy13(SB), $40-8
	MOVQ x+0(FP), AX
	MOVQ 0(AX), DX
	MOVQ 8(AX), CX
	MOVQ 16(AX), BX
	MOVQ 24(AX), SI
	MOVQ 32(AX), DI
	MOVQ 40(AX), R8
	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R15,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP))
	REDUCE(DX,CX,BX,SI,DI,R8,R15,s0-8(SP),s1-16(SP),s2-24(SP),s3-32(SP),s4-40(SP))

	MOVQ DX, R15
	MOVQ CX, s0-8(SP)
	MOVQ BX, s1-16(SP)
	MOVQ SI, s2-24(SP)
	MOVQ DI, s3-32(SP)
	MOVQ R8, s4-40(SP)
	ADDQ DX, DX
	ADCQ CX, CX
	ADCQ BX, BX
	ADCQ SI, SI
	ADCQ DI, DI
	ADCQ R8, R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	ADDQ R15, DX
	ADCQ s0-8(SP), CX
	ADCQ s1-16(SP), BX
	ADCQ s2-24(SP), SI
	ADCQ s3-32(SP), DI
	ADCQ s4-40(SP), R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	ADDQ 0(AX), DX
	ADCQ 8(AX), CX
	ADCQ 16(AX), BX
	ADCQ 24(AX), SI
	ADCQ 32(AX), DI
	ADCQ 40(AX), R8

	// reduce element(DX,CX,BX,SI,DI,R8) using temp registers (R9,R10,R11,R12,R13,R14)
	REDUCE(DX,CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14)

	MOVQ DX, 0(AX)
	MOVQ CX, 8(AX)
	MOVQ BX, 16(AX)
	MOVQ SI, 24(AX)
	MOVQ DI, 32(AX)
	MOVQ R8, 40(AX)
	RET

// Butterfly(a, b *Element) sets a = a + b; b = a - b
TEXT ·Butterfly(SB), $48-16
	MOVQ    a+0(FP), AX
	MOVQ    0(AX), CX
	MOVQ    8(AX), BX
	MOVQ    16(AX), SI
	MOVQ    24(AX), DI
	MOVQ    32(AX), R8
	MOVQ    40(AX), R9
	MOVQ    CX, R10
	MOVQ    BX, R11
	MOVQ    SI, R12
	MOVQ    DI, R13
	MOVQ    R8, R14
	MOVQ    R9, R15
	XORQ    AX, AX
	MOVQ    b+8(FP), DX
	ADDQ    0(DX), CX
	ADCQ    8(DX), BX
	ADCQ    16(DX), SI
	ADCQ    24(DX), DI
	ADCQ    32(DX), R8
	ADCQ    40(DX), R9
	SUBQ    0(DX), R10
	SBBQ    8(DX), R11
	SBBQ    16(DX), R12
	SBBQ    24(DX), R13
	SBBQ    32(DX), R14
	SBBQ    40(DX), R15
	MOVQ    CX, s0-8(SP)
	MOVQ    BX, s1-16(SP)
	MOVQ    SI, s2-24(SP)
	MOVQ    DI, s3-32(SP)
	MOVQ    R8, s4-40(SP)
	MOVQ    R9, s5-48(SP)
	MOVQ    $0x8508c00000000001, CX
	MOVQ    $0x170b5d4430000000, BX
	MOVQ    $0x1ef3622fba094800, SI
	MOVQ    $0x1a22d9f300f5138f, DI
	MOVQ    $0xc63b05c06ca1493b, R8
	MOVQ    $0x01ae3a4617c510ea, R9
	CMOVQCC AX, CX
	CMOVQCC AX, BX
	CMOVQCC AX, SI
	CMOVQCC AX, DI
	CMOVQCC AX, R8
	CMOVQCC AX, R9
	ADDQ    CX, R10
	ADCQ    BX, R11
	ADCQ    SI, R12
	ADCQ    DI, R13
	ADCQ    R8, R14
	ADCQ    R9, R15
	MOVQ    s0-8(SP), CX
	MOVQ    s1-16(SP), BX
	MOVQ    s2-24(SP), SI
	MOVQ    s3-32(SP), DI
	MOVQ    s4-40(SP), R8
	MOVQ    s5-48(SP), R9
	MOVQ    R10, 0(DX)
	MOVQ    R11, 8(DX)
	MOVQ    R12, 16(DX)
	MOVQ    R13, 24(DX)
	MOVQ    R14, 32(DX)
	MOVQ    R15, 40(DX)

	// reduce element(CX,BX,SI,DI,R8,R9) using temp registers (R10,R11,R12,R13,R14,R15)
	REDUCE(CX,BX,SI,DI,R8,R9,R10,R11,R12,R13,R14,R15)

	MOVQ a+0(FP), AX
	MOVQ CX, 0(AX)
	MOVQ BX, 8(AX)
	MOVQ SI, 16(AX)
	MOVQ DI, 24(AX)
	MOVQ R8, 32(AX)
	MOVQ R9, 40(AX)
	RET
