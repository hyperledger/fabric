//go:build !noadx && !purego

package cpu

import "golang.org/x/sys/cpu"

var (
	SupportADX = cpu.X86.HasADX && cpu.X86.HasBMI2
)
