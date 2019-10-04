// +build ppc64le,linux

package terminal

// Used syscall numbers from https://github.com/golang/go/blob/master/src/syscall/ztypes_linux_ppc64le.go
const ioctlReadTermios = 0x402c7413  // syscall.TCGETS
const ioctlWriteTermios = 0x802c7414 // syscall.TCSETS
