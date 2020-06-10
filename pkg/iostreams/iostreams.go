package iostreams

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
)

type IOStreams struct {
	In     io.ReadCloser
	Out    io.Writer
	ErrOut io.Writer
}

func System() *IOStreams {
	return &IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return &IOStreams{
		In:     ioutil.NopCloser(in),
		Out:    out,
		ErrOut: errOut,
	}, in, out, errOut
}
