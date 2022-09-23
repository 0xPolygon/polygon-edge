//+build amd64 AND !go1.13

#include "textflag.h"

// func feMul(outp *uint64, xp *uint64, yp *uint64)
TEXT ·feMul(SB),NOSPLIT,$0
	MOVQ outp+0(FP), DI
	MOVQ xp+8(FP), BX
	MOVQ yp+16(FP), CX

	// Calculate r0
	MOVQ 0(BX), AX     // rax <-- x0
	MULQ 0(CX)         // rdx, rax <-- x0*y0
	MOVQ AX, SI        // r00 = rax
	MOVQ DX, BP        // r01 = rdx

	MOVQ 8(BX), DX     // rdx <-- x1
	IMUL3Q $19, DX, AX // rax <-- x1*19
	MULQ 32(CX)        // rdx, rax <-- x1_19*y4
	ADDQ AX, SI        // r00 += rax
	ADCQ DX, BP        // r01 += rdx

	MOVQ 16(BX), DX    // rdx <-- x2
	IMUL3Q $19, DX, AX // rax <-- x2*19
	MULQ 24(CX)        // rdx, rax <-- x2_19*y3
	ADDQ AX, SI        // r00 += rax
	ADCQ DX, BP        // r01 += rdx

	MOVQ 24(BX), DX    // rdx <-- x3
	IMUL3Q $19, DX, AX // rax <-- x3*19
	MULQ 16(CX)        // rdx, rax <-- x3_19 * y2
	ADDQ AX, SI        // r00 += rax
	ADCQ DX, BP        // r01 += rdx

	MOVQ 32(BX), DX    // rdx <-- x4
	IMUL3Q $19, DX, AX // rax <-- x4*19
	MULQ 8(CX)         // rdx rax <-- x4_19*y1
	ADDQ AX, SI        // r00 += rax
	ADCQ DX, BP        // r01 += rdx

	// Calculate r1
	MOVQ 0(BX), AX
	MULQ 8(CX)
	MOVQ AX, R8 // r10
	MOVQ DX, R9 // r11

	MOVQ 8(BX), AX
	MULQ 0(CX)
	ADDQ AX, R8
	ADCQ DX, R9

	MOVQ 16(BX), DX
	IMUL3Q $19, DX, AX
	MULQ 32(CX)
	ADDQ AX, R8
	ADCQ DX, R9

	MOVQ 24(BX), DX
	IMUL3Q $19, DX, AX
	MULQ 24(CX)
	ADDQ AX, R8
	ADCQ DX, R9

	MOVQ 32(BX), DX
	IMUL3Q $19, DX, AX
	MULQ 16(CX)
	ADDQ AX, R8
	ADCQ DX, R9

	// Calculate r2
	MOVQ 0(BX), AX
	MULQ 16(CX)
	MOVQ AX, R10 // r20
	MOVQ DX, R11 // r21

	MOVQ 8(BX), AX
	MULQ 8(CX)
	ADDQ AX, R10
	ADCQ DX, R11

	MOVQ 16(BX), AX
	MULQ 0(CX)
	ADDQ AX, R10
	ADCQ DX, R11

	MOVQ 24(BX), DX
	IMUL3Q $19, DX, AX
	MULQ 32(CX)
	ADDQ AX, R10
	ADCQ DX, R11

	MOVQ 32(BX), DX
	IMUL3Q $19, DX, AX
	MULQ 24(CX)
	ADDQ AX, R10
	ADCQ DX, R11

	// Calculate r3
	MOVQ 0(BX), AX
	MULQ 24(CX)
	MOVQ AX, R12 // r30
	MOVQ DX, R13 // r31

	MOVQ 8(BX), AX
	MULQ 16(CX)
	ADDQ AX, R12
	ADCQ DX, R13

	MOVQ 16(BX), AX
	MULQ 8(CX)
	ADDQ AX, R12
	ADCQ DX, R13

	MOVQ 24(BX), AX
	MULQ 0(CX)
	ADDQ AX, R12
	ADCQ DX, R13

	MOVQ 32(BX), DX
	IMUL3Q $19, DX, AX
	MULQ 32(CX)
	ADDQ AX, R12
	ADCQ DX, R13

	// Calculate r4
	MOVQ 0(BX), AX
	MULQ 32(CX)
	MOVQ AX, R14 // r40
	MOVQ DX, R15 // r41

	MOVQ 8(BX), AX
	MULQ 24(CX)
	ADDQ AX, R14
	ADCQ DX, R15

	MOVQ 16(BX), AX
	MULQ 16(CX)
	ADDQ AX, R14
	ADCQ DX, R15

	MOVQ 24(BX), AX
	MULQ 8(CX)
	ADDQ AX, R14
	ADCQ DX, R15

	MOVQ 32(BX), AX
	MULQ 0(CX)
	ADDQ AX, R14
	ADCQ DX, R15


	MOVQ $2251799813685247, AX // (1<<51) - 1
	SHLQ $13, SI, BP     // r01 = shld with r00
	ANDQ AX, SI          // r00 &= mask51
	SHLQ $13, R8, R9     // r11 = shld with r10
	ANDQ AX, R8          // r10 &= mask51
	ADDQ BP, R8          // r10 += r01
	SHLQ $13, R10, R11   // r21 = shld with r20
	ANDQ AX, R10         // r20 &= mask51
	ADDQ R9, R10         // r20 += r11
	SHLQ $13, R12, R13   // r31 = shld with r30
	ANDQ AX, R12         // r30 &= mask51
	ADDQ R11, R12        // r30 += r21
	SHLQ $13, R14, R15   // r41 = shld with r40
	ANDQ AX, R14         // r40 &= mask51
	ADDQ R13, R14        // r40 += r31
	IMUL3Q $19, R15, R15 // r41 = r41*19
	ADDQ R15, SI         // r00 += r41

	MOVQ SI, DX          // rdx <-- r00
	SHRQ $51, DX         // rdx <-- r00 >> 51
	ADDQ DX, R8          // r10 += r00 >> 51
	MOVQ R8, DX          // rdx <-- r10
	SHRQ $51, DX         // rdx <-- r10 >> 51
	ANDQ AX, SI          // r00 &= mask51
	ADDQ DX, R10         // r20 += r10 >> 51
	MOVQ R10, DX         // rdx <-- r20
	SHRQ $51, DX         // rdx <-- r20 >> 51
	ANDQ AX, R8          // r10 &= mask51
	ADDQ DX, R12         // r30 += r20 >> 51
	MOVQ R12, DX         // rdx <-- r30
	SHRQ $51, DX         // rdx <-- r30 >> 51
	ANDQ AX, R10         // r20 &= mask51
	ADDQ DX, R14         // r40 += r30 >> 51
	MOVQ R14, DX         // rdx <-- r40
	SHRQ $51, DX         // rdx <-- r40 >> 51
	ANDQ AX, R12         // r30 &= mask51
	IMUL3Q $19, DX, DX   // rdx <-- (r40 >> 51) * 19
	ADDQ DX, SI          // r00 += (r40 >> 51) *19
	ANDQ AX, R14         // r40 &= mask51

	MOVQ SI, 0(DI)
	MOVQ R8, 8(DI)
	MOVQ R10, 16(DI)
	MOVQ R12, 24(DI)
	MOVQ R14, 32(DI)
	RET

// func feSquare(outp *uint64, xp *uint64)
TEXT ·feSquare(SB),NOSPLIT,$0
    MOVQ outp+0(FP), DI
    MOVQ xp+8(FP), SI

    // r0 = x0*x0 + x1*38*x4 + x2*38*x3
    MOVQ 0(SI), AX
    MULQ 0(SI)
    MOVQ AX, CX // r00
    MOVQ DX, R8 // r01

    MOVQ 8(SI), DX
    IMUL3Q $38, DX, AX
    MULQ 32(SI)
    ADDQ AX, CX
    ADCQ DX, R8

    MOVQ 16(SI), DX
    IMUL3Q $38, DX, AX
    MULQ 24(SI)
    ADDQ AX, CX
    ADCQ DX, R8

    // r1 = x0*2*x1 + x2*38*x4 + x3*19*x3
    MOVQ 0(SI), AX
    SHLQ $1, AX
    MULQ 8(SI)
    MOVQ AX, R9  // r10
    MOVQ DX, R10 // r11

    MOVQ 16(SI), DX
    IMUL3Q $38, DX, AX
    MULQ 32(SI)
    ADDQ AX, R9
    ADCQ DX, R10

    MOVQ 24(SI), DX
    IMUL3Q $19, DX, AX
    MULQ 24(SI)
    ADDQ AX, R9
    ADCQ DX, R10

    // r2 = x0*2*x2 + x1*x1 + x3*38*x4
    MOVQ 0(SI), AX
    SHLQ $1, AX
    MULQ 16(SI)
    MOVQ AX, R11 // r20
    MOVQ DX, R12 // r21

    MOVQ 8(SI), AX
    MULQ 8(SI)
    ADDQ AX, R11
    ADCQ DX, R12

    MOVQ 24(SI), DX
    IMUL3Q $38, DX, AX
    MULQ 32(SI)
    ADDQ AX, R11
    ADCQ DX, R12

    // r3 = x0*2*x3 + x1*2*x2 + x4*19*x4
    MOVQ 0(SI), AX
    SHLQ $1, AX
    MULQ 24(SI)
    MOVQ AX, R13 // r30
    MOVQ DX, R14 // r31

    MOVQ 8(SI), AX
    SHLQ $1, AX
    MULQ 16(SI)
    ADDQ AX, R13
    ADCQ DX, R14

    MOVQ 32(SI), DX
    IMUL3Q $19, DX, AX
    MULQ 32(SI)
    ADDQ AX, R13
    ADCQ DX, R14

    // r4 = x0*2*x4 + x1*2*x3 + x2*x2
    MOVQ 0(SI), AX
    SHLQ $1, AX
    MULQ 32(SI)
    MOVQ AX, R15 // r40
    MOVQ DX, BX  // r41

    MOVQ 8(SI), AX
    SHLQ $1, AX
    MULQ 24(SI)
    ADDQ AX, R15
    ADCQ DX, BX

    MOVQ 16(SI), AX
    MULQ 16(SI)
    ADDQ AX, R15
    ADCQ DX, BX

    // Reduce
    MOVQ $2251799813685247, AX // (1<<51) - 1
    SHLQ $13, CX, R8     // r01 = shld with r00
    ANDQ AX, CX          // r00 &= mask51
    SHLQ $13, R9, R10    // r11 = shld with r10
    ANDQ AX, R9          // r10 &= mask51
    ADDQ R8, R9          // r10 += r01
    SHLQ $13, R11, R12   // r21 = shld with r20
    ANDQ AX, R11         // r20 &= mask51
    ADDQ R10, R11        // r20 += r11
    SHLQ $13, R13, R14   // r31 = shld with r30
    ANDQ AX, R13         // r30 &= mask51
    ADDQ R12, R13        // r30 += r21
    SHLQ $13, R15, BX    // r41 = shld with r40
    ANDQ AX, R15         // r40 &= mask51
    ADDQ R14, R15        // r40 += r31
    IMUL3Q $19, BX, DX   // r41 = r41*19
    ADDQ DX, CX          // r00 += r41

    MOVQ CX, DX          // rdx <-- r00
    SHRQ $51, DX         // rdx <-- r00 >> 51
    ADDQ DX, R9          // r10 += r00 >> 51
    MOVQ R9, DX          // rdx <-- r10
    SHRQ $51, DX         // rdx <-- r10 >> 51
    ANDQ AX, CX          // r00 &= mask51
    ADDQ DX, R11         // r20 += r10 >> 51
    MOVQ R11, DX         // rdx <-- r20
    SHRQ $51, DX         // rdx <-- r20 >> 51
    ANDQ AX, R9          // r10 &= mask51
    ADDQ DX, R13         // r30 += r20 >> 51
    MOVQ R13, DX         // rdx <-- r30
    SHRQ $51, DX         // rdx <-- r30 >> 51
    ANDQ AX, R11         // r20 &= mask51
    ADDQ DX, R15         // r40 += r30 >> 51
    MOVQ R15, DX         // rdx <-- r40
    SHRQ $51, DX         // rdx <-- r40 >> 51
    ANDQ AX, R13         // r30 &= mask51
    IMUL3Q $19, DX, DX   // rdx <-- (r40 >> 51) * 19
    ADDQ DX, CX          // r00 += (r40 >> 51) *19
    ANDQ AX, R15         // r40 &= mask51

    MOVQ CX, 0(DI)
    MOVQ R9, 8(DI)
    MOVQ R11, 16(DI)
    MOVQ R13, 24(DI)
    MOVQ R15, 32(DI)
    RET
