package mocks

import (
	"io"

	"google.golang.org/grpc"
)

// ServerStreamClient is a mock which satisfies the grpc.ClientStream interface,
// allowing it to be returned by methods where the server returns a stream of
// results. It can be populated with a list of results to return, or an error
// to return.
type ServerStreamClient[T any] struct {
	grpc.ClientStream
	Results []*T
	Err     error
}

// Recv returns the error, if populated. Otherwise it returns the next item from
// the list of results. If it has returned all items already, it returns EOF.
func (c *ServerStreamClient[T]) Recv() (*T, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	if len(c.Results) == 0 {
		return nil, io.EOF
	}
	res := c.Results[0]
	c.Results = c.Results[1:]
	return res, nil
}
