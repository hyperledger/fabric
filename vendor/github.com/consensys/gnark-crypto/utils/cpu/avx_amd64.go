//go:build !noavx && !purego

package cpu

import "golang.org/x/sys/cpu"

var (
	SupportAVX512 = SupportADX && cpu.X86.HasAVX512 && cpu.X86.HasAVX512DQ && cpu.X86.HasAVX512VBMI2
)
