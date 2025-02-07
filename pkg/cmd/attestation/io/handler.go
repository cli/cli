package io

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
)

type Handler struct {
	ColorScheme  *iostreams.ColorScheme
	IO           *iostreams.IOStreams
	debugEnabled bool
}

func NewHandler(io *iostreams.IOStreams) *Handler {
	enabled, _ := utils.IsDebugEnabled()

	return &Handler{
		ColorScheme:  io.ColorScheme(),
		IO:           io,
		debugEnabled: enabled,
	}
}

func NewTestHandler() *Handler {
	testIO, _, _, _ := iostreams.Test()
	return NewHandler(testIO)
}

// Printf writes the formatted arguments to the stderr writer.
func (h *Handler) Printf(f string, v ...interface{}) (int, error) {
	if !h.IO.IsStdoutTTY() {
		return 0, nil
	}
	return fmt.Fprintf(h.IO.ErrOut, f, v...)
}

func (h *Handler) OutPrintf(f string, v ...interface{}) (int, error) {
	return fmt.Fprintf(h.IO.Out, f, v...)
}

// Println writes the arguments to the stderr writer with a newline at the end.
func (h *Handler) Println(v ...interface{}) (int, error) {
	if !h.IO.IsStdoutTTY() {
		return 0, nil
	}
	return fmt.Fprintln(h.IO.ErrOut, v...)
}

func (h *Handler) OutPrintln(v ...interface{}) (int, error) {
	return fmt.Fprintln(h.IO.Out, v...)
}

func (h *Handler) VerbosePrint(msg string) (int, error) {
	if !h.debugEnabled || !h.IO.IsStdoutTTY() {
		return 0, nil
	}

	return fmt.Fprintln(h.IO.ErrOut, msg)
}

func (h *Handler) VerbosePrintf(f string, v ...interface{}) (int, error) {
	if !h.debugEnabled || !h.IO.IsStdoutTTY() {
		return 0, nil
	}
	return fmt.Fprintf(h.IO.ErrOut, f, v...)
}

func (h *Handler) PrintBulletPoints(rows [][]string) (int, error) {
	if !h.IO.IsStdoutTTY() {
		return 0, nil
	}
	maxColLen := 0
	for _, row := range rows {
		if len(row[0]) > maxColLen {
			maxColLen = len(row[0])
		}
	}

	info := ""
	for _, row := range rows {
		dots := strings.Repeat(".", maxColLen-len(row[0]))
		info += fmt.Sprintf("%s:%s %s\n", row[0], dots, row[1])
	}
	return fmt.Fprintln(h.IO.ErrOut, info)
}
