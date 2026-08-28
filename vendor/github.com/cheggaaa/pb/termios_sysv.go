// +build linux solaris aix zos
// +build !appengine

package pb

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TCGETS
const ioctlWriteTermios = unix.TCSETS
