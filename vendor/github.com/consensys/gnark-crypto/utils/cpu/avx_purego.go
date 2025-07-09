//go:build noavx || purego || !amd64

package cpu

const SupportAVX512 = false
