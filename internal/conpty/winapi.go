//go:build windows
// +build windows

package conpty

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x20016
)

var (
	modkernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procCreatePseudoConsole = modkernel32.NewProc("CreatePseudoConsole")
	procClosePseudoConsole  = modkernel32.NewProc("ClosePseudoConsole")
	procResizePseudoConsole = modkernel32.NewProc("ResizePseudoConsole")
)

// HRESULT WINAPI CreatePseudoConsole(
//     _In_ COORD size,
//     _In_ HANDLE hInput,
//     _In_ HANDLE hOutput,
//     _In_ DWORD dwFlags,
//     _Out_ HPCON* phPC
// );
//
// sys createPseudoConsole(size uint32, hInput windows.Handle, hOutput windows.Handle, dwFlags uint32, hpcon *windows.Handle) (hr error) = kernel32.CreatePseudoConsole
// CreatePseudoConsole creates a windows pseudo console.
func CreatePseudoConsole(size windows.Coord, hInput windows.Handle, hOutput windows.Handle, dwFlags uint32, hpcon *windows.Handle) error {
	// We need this wrapper as the function takes a COORD struct and not a pointer to one, so we need to cast to something beforehand.
	return createPseudoConsole(*((*uint32)(unsafe.Pointer(&size))), hInput, hOutput, 0, hpcon)
}

// HRESULT WINAPI ResizePseudoConsole(
//     _In_ HPCON hPC ,
//     _In_ COORD size
// );
//
// sys resizePseudoConsole(hPc windows.Handle, size uint32) (hr error) = kernel32.ResizePseudoConsole
// ResizePseudoConsole resizes the internal buffers of the pseudo console to the width and height specified in `size`.
func ResizePseudoConsole(hpcon windows.Handle, size windows.Coord) error {
	// We need this wrapper as the function takes a COORD struct and not a pointer to one, so we need to cast to something beforehand.
	return resizePseudoConsole(hpcon, *((*uint32)(unsafe.Pointer(&size))))
}

// void WINAPI ClosePseudoConsole(
//     _In_ HPCON hPC
// );
//
// sys ClosePseudoConsole(hpc windows.Handle) = kernel32.ClosePseudoConsole
// ClosePseudoConsole closes the pseudo console.
func ClosePseudoConsole(hpc windows.Handle) {
	syscall.Syscall(procClosePseudoConsole.Addr(), 1, uintptr(hpc), 0, 0)
	return
}

func createPseudoConsole(size uint32, hInput windows.Handle, hOutput windows.Handle, dwFlags uint32, hpcon *windows.Handle) (hr error) {
	r0, _, _ := syscall.Syscall6(procCreatePseudoConsole.Addr(), 5, uintptr(size), uintptr(hInput), uintptr(hOutput), uintptr(dwFlags), uintptr(unsafe.Pointer(hpcon)), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}

func resizePseudoConsole(hPc windows.Handle, size uint32) (hr error) {
	r0, _, _ := syscall.Syscall(procResizePseudoConsole.Addr(), 2, uintptr(hPc), uintptr(size), 0)
	if int32(r0) < 0 {
		if r0&0x1fff0000 == 0x00070000 {
			r0 &= 0xffff
		}
		hr = syscall.Errno(r0)
	}
	return
}
