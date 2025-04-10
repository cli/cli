package grpctunnel

import "io"

// rwio is an utility to combine the closing of a R/W interface on a single operation.
type rwio struct {
	io.ReadCloser
	io.WriteCloser
}

func (s *rwio) Close() error {
	werr := s.WriteCloser.Close()
	rerr := s.ReadCloser.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

func (s *rwio) CloseWrite() error {
	return s.WriteCloser.Close()
}
