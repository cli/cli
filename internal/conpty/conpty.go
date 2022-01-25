//go:build windows
// +build windows

package conpty

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	errClosedConPty   = errors.New("pseudo console is closed")
	errNotInitialized = errors.New("pseudo console hasn't been initialized")
)

// Pty is a wrapper around a Windows PseudoConsole handle. Create a new instance by calling `Create()`.
type Pty struct {
	// handleLock guards hpc
	handleLock sync.RWMutex
	// hpc is the pseudo console handle
	hpc windows.Handle
	// inPipe and outPipe are our end of the pipes to read/write to the pseudo console.
	inPipe  *os.File
	outPipe *os.File
}

// Create returns a new `Pty` object. This object is not ready for IO until `UpdateProcThreadAttribute` is called and a process has been started.
func Create(width, height int16, flags uint32) (*Pty, error) {
	// First we need to make both ends of the conpty's pipes, two to get passed into a process to use as input/output, and two for us to keep to
	// make use of this data.
	ptyIn, inPipeOurs, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipes for pseudo console: %w", err)
	}

	outPipeOurs, ptyOut, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipes for pseudo console: %w", err)
	}

	var hpc windows.Handle
	coord := windows.Coord{X: width, Y: height}
	err = CreatePseudoConsole(coord, windows.Handle(ptyIn.Fd()), windows.Handle(ptyOut.Fd()), 0, &hpc)
	if err != nil {
		return nil, fmt.Errorf("failed to create pseudo console: %w", err)
	}

	// The pty's end of its pipes can be closed here without worry. They're duped into the conhost
	// that will be launched and will be released on a call to ClosePseudoConsole() (Close() on the Pty object).
	if err := ptyOut.Close(); err != nil {
		return nil, fmt.Errorf("failed to close pseudo console handle: %w", err)
	}
	if err := ptyIn.Close(); err != nil {
		return nil, fmt.Errorf("failed to close pseudo console handle: %w", err)
	}

	return &Pty{
		hpc:     hpc,
		inPipe:  inPipeOurs,
		outPipe: outPipeOurs,
	}, nil
}

// UpdateProcThreadAttribute updates the passed in attribute list to contain the entry necessary for use with
// CreateProcess.
func (c *Pty) UpdateProcThreadAttribute(attrList *windows.ProcThreadAttributeListContainer) error {
	c.handleLock.RLock()
	defer c.handleLock.RUnlock()

	if c.hpc == 0 {
		return errClosedConPty
	}

	if err := attrList.Update(
		PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		unsafe.Pointer(c.hpc),
		unsafe.Sizeof(c.hpc),
	); err != nil {
		return fmt.Errorf("failed to update proc thread attributes for pseudo console: %w", err)
	}

	return nil
}

// Resize resizes the internal buffers of the pseudo console to the passed in size
func (c *Pty) Resize(width, height int16) error {
	c.handleLock.RLock()
	defer c.handleLock.RUnlock()

	if c.hpc == 0 {
		return errClosedConPty
	}

	coord := windows.Coord{X: width, Y: height}
	if err := ResizePseudoConsole(c.hpc, coord); err != nil {
		return fmt.Errorf("failed to resize pseudo console: %w", err)
	}
	return nil
}

// Close closes the pseudo-terminal and cleans up all attached resources
func (c *Pty) Close() error {
	c.handleLock.Lock()
	defer c.handleLock.Unlock()

	if c.hpc == 0 {
		return errClosedConPty
	}

	// Close the pseudo console, set the handle to 0 to invalidate this object and then close the side of the pipes that we own.
	ClosePseudoConsole(c.hpc)
	c.hpc = 0
	if err := c.inPipe.Close(); err != nil {
		return fmt.Errorf("failed to close pseudo console input pipe: %w", err)
	}
	if err := c.outPipe.Close(); err != nil {
		return fmt.Errorf("failed to close pseudo console output pipe: %w", err)
	}
	return nil
}

// OutPipe returns the output pipe of the pseudo console.
func (c *Pty) OutPipe() *os.File {
	return c.outPipe
}

// InPipe returns the input pipe of the pseudo console.
func (c *Pty) InPipe() *os.File {
	return c.inPipe
}

// Write writes the contents of `buf` to the pseudo console. Returns the number of bytes written and an error if there is one.
func (c *Pty) Write(buf []byte) (int, error) {
	if c.inPipe == nil {
		return 0, errNotInitialized
	}
	return c.inPipe.Write(buf)
}

// Read reads from the pseudo console into `buf`. Returns the number of bytes read and an error if there is one.
func (c *Pty) Read(buf []byte) (int, error) {
	if c.outPipe == nil {
		return 0, errNotInitialized
	}
	return c.outPipe.Read(buf)
}
