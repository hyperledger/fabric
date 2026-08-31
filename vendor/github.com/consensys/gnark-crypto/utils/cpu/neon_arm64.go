//go:build !noneon && !purego

package cpu

import "golang.org/x/sys/cpu"

var SupportNEON = cpu.ARM64.HasASIMD
