package grpctunnel

//go:generate moq -out mock_client_test.go . client
//go:generate moq -out mock_withprogress_test.go . withProgresss
//go:generate moq -out mock_invoker_test.go . invoker
//go:generate moq -out mock_portforwarder_test.go -pkg grpctunnel ../portforwarder PortForwarder

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
)

// Define sentinel errors for test scenarios
var (
	errInvokerCreation = errors.New("failed to create invoker")
	errStartSSHServer  = errors.New("failed to start ssh server")
	errForwardPort     = errors.New("failed to forward port")
	errConnectToPort   = errors.New("failed to connect to forwarded port")
	errTunnelClosed    = errors.New("tunnel was closed")
	errCommandFailed   = errors.New("command failed")
)

func TestConnectWithOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    rpc.StartSSHServerOptions
		port    int
		profile string
		err     error
	}{
		{
			name: "success",
			opts: rpc.StartSSHServerOptions{
				UserPublicKeyFile: "test-key.pub",
			},
			port:    2222,
			profile: "testuser",
			err:     nil,
		},
		{
			name:    "error creating invoker",
			opts:    rpc.StartSSHServerOptions{},
			port:    0,
			profile: "",
			err:     errInvokerCreation,
		},
		{
			name:    "error starting ssh server",
			opts:    rpc.StartSSHServerOptions{},
			port:    0,
			profile: "",
			err:     errStartSSHServer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create a mock port forwarder
			mockPortForwarder := &PortForwarderMock{
				ForwardPortFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts) error {
					return nil
				},
				ForwardPortToListenerFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts, listener *net.TCPListener) error {
					return nil
				},
			}

			// Set up the tunnel with the mock port forwarder
			tunnel := &Tunnel{
				fwd:    mockPortForwarder,
				closed: make(chan error, 1),
				port:   0,
			}

			// Mock rpc.CreateInvoker by patching the package-level function
			originalCreateInvoker := createInvoker
			defer func() { createInvoker = originalCreateInvoker }()

			// Using the correct function signature
			createInvoker = func(ctx context.Context, fwd portforwarder.PortForwarder) (rpc.Invoker, error) {
				if errors.Is(tt.err, errInvokerCreation) {
					return nil, errInvokerCreation
				}

				// Create a simple mock invoker for the success case
				mockInvoker := &InvokerMock{
					StartSSHServerWithOptionsFunc: func(ctx context.Context, options rpc.StartSSHServerOptions) (int, string, error) {
						// Verify that the options passed match the expected options
						if options.UserPublicKeyFile != tt.opts.UserPublicKeyFile {
							t.Fatalf("Expected UserPublicKeyFile %q, got %q", tt.opts.UserPublicKeyFile, options.UserPublicKeyFile)
						}
						return tt.port, tt.profile, tt.err
					},
					CloseFunc: func() error {
						return nil
					},
				}

				return mockInvoker, nil
			}

			// Call the method under test
			err := tunnel.ConnectWithOptions(ctx, tt.opts)

			if !errors.Is(err, tt.err) {
				t.Fatalf("Expected error %v, got %v", tt.err, err)
			}
		})
	}
}

func TestProxyStdio(t *testing.T) {
	tests := []struct {
		name             string
		forwardPortErr   error
		connectToPortErr error
		connectCalls     int
		expectedErr      error
	}{
		{
			name:             "success",
			forwardPortErr:   nil,
			connectCalls:     1,
			connectToPortErr: nil,
			expectedErr:      nil,
		},
		{
			name:             "error forwarding port",
			forwardPortErr:   errForwardPort,
			connectCalls:     0,
			connectToPortErr: nil,
			expectedErr:      errForwardPort,
		},
		{
			name:             "error connecting to port",
			forwardPortErr:   nil,
			connectCalls:     1,
			connectToPortErr: errConnectToPort,
			expectedErr:      errConnectToPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create a mock port forwarder
			mockForwarder := &PortForwarderMock{
				ForwardPortFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts) error {
					return tt.forwardPortErr
				},
				ConnectToForwardedPortFunc: func(ctx context.Context, conn io.ReadWriteCloser, opts portforwarder.ForwardPortOpts) error {
					// Verify that the options are passed correctly - should match what's returned by ForwardOpts()
					if opts.Port != 2222 || !opts.Internal || !opts.KeepAlive {
						t.Fatalf("Expected options %+v, got %+v", portforwarder.ForwardPortOpts{
							Port:      2222,
							Internal:  true,
							KeepAlive: true,
						}, opts)
					}
					return tt.connectToPortErr
				},
			}

			// Create a mock invoker
			mockInvoker := &InvokerMock{
				CloseFunc: func() error {
					return nil
				},
				KeepAliveFunc: func() {
					// Do nothing
				},
			}

			// Set up the tunnel with the mock port forwarder, this is equivalent to
			// having called `ConnectWithOptions` before.
			tunnel := &Tunnel{
				fwd: mockForwarder,
				invoker: &sshInvoker{
					port:    2222,
					user:    "testuser",
					Invoker: mockInvoker,
				},
				closed: make(chan error, 1),
			}

			err := tunnel.ProxyStdio(ctx)

			if !errors.Is(err, tt.expectedErr) {
				t.Fatalf("ProxyStdio() error = %v, want %v", err, tt.expectedErr)
			}

			// Verify ForwardPort was called
			if len(mockForwarder.ForwardPortCalls()) != 1 {
				t.Fatalf("ForwardPort was called %d times, expected 1", len(mockForwarder.ForwardPortCalls()))
			}

			// Verify ConnectToForwardedPort was called if ForwardPort succeeded
			if len(mockForwarder.ConnectToForwardedPortCalls()) != tt.connectCalls {
				t.Fatalf("ConnectToForwardedPort was called %d times, expected %d",
					len(mockForwarder.ConnectToForwardedPortCalls()),
					tt.connectCalls)
			}
		})
	}
}

func TestForwardAndRun(t *testing.T) {
	tests := []struct {
		name           string
		forwardErr     error
		buildForwarder func(cmdChan chan error, fwdChan chan error) *PortForwarderMock
		buildCmd       func(cmdChan chan error, fwdChan chan error) func(string, int) error
		expectedErr    error
	}{
		{
			name:       "success when command finishes",
			forwardErr: nil,
			buildForwarder: func(cmdChan chan error, fwdChan chan error) *PortForwarderMock {
				return &PortForwarderMock{
					ForwardPortToListenerFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts, listener *net.TCPListener) error {
						defer close(fwdChan)
						// Wait for cmd to complete before returning
						<-cmdChan
						return nil
					},
					CloseFunc: func() error {
						return nil
					},
				}
			},
			buildCmd: func(cmdChan chan error, fwdChan chan error) func(string, int) error {
				return func(dest string, port int) error {
					defer close(cmdChan)
					// Return immediately with success
					return nil
				}
			},
			expectedErr: nil,
		},
		{
			name:       "error when tunnel closed",
			forwardErr: errTunnelClosed,
			buildForwarder: func(cmdChan chan error, fwdChan chan error) *PortForwarderMock {
				return &PortForwarderMock{
					ForwardPortToListenerFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts, listener *net.TCPListener) error {
						defer close(fwdChan)
						// Return immediately with an error
						return errTunnelClosed
					},
					CloseFunc: func() error {
						return nil
					},
				}
			},
			buildCmd: func(cmdChan chan error, fwdChan chan error) func(string, int) error {
				return func(dest string, port int) error {
					defer close(cmdChan)
					// Wait for forwarder to complete before returning
					<-fwdChan
					return nil
				}
			},
			expectedErr: errTunnelClosed,
		},
		{
			name:       "error when command fails",
			forwardErr: nil,
			buildForwarder: func(cmdChan chan error, fwdChan chan error) *PortForwarderMock {
				return &PortForwarderMock{
					ForwardPortToListenerFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts, listener *net.TCPListener) error {
						defer close(fwdChan)
						// Wait for cmd to complete before returning
						<-cmdChan
						return nil
					},
					CloseFunc: func() error {
						return nil
					},
				}
			},
			buildCmd: func(cmdChan chan error, fwdChan chan error) func(string, int) error {
				return func(dest string, port int) error {
					defer close(cmdChan)
					// Return immediately with an error
					return errCommandFailed
				}
			},
			expectedErr: errCommandFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup channels to coordinate timing between command and forwarder
			cmdChan := make(chan error, 1)
			fwdChan := make(chan error, 1)

			// Create a mock port forwarder using the builder function from the test case
			mockForwarder := tt.buildForwarder(cmdChan, fwdChan)

			// Create a mock invoker
			mockInvoker := &InvokerMock{
				CloseFunc: func() error {
					return nil
				},
				KeepAliveFunc: func() {
					// Do nothing
				},
			}

			// Setup the tunnel with the mock port forwarder and wrap the invoker in
			// an sshInvoker. This is the equivalent of calling ConnectWithOptions
			tunnel := &Tunnel{
				fwd: mockForwarder,
				invoker: &sshInvoker{
					port:    0, // Port will be assigned by Forward
					user:    "testuser",
					Invoker: mockInvoker,
				},
				closed: make(chan error, 1),
			}

			// Use the Forward method to set up the tunnel
			if err := tunnel.Forward(ctx, 0); err != nil {
				t.Fatalf("Failed to forward: %v", err)
			}

			// Create the command function using the builder from the test case
			cmdFunc := tt.buildCmd(cmdChan, fwdChan)

			// Wrap the cmdFunc to validate parameters
			cmdWithAssertions := func(dest string, port int) error {
				// Verify the parameters passed to the command function
				if dest != "testuser@localhost" {
					t.Errorf("Expected destination %q, got %q", "testuser@localhost", dest)
				}
				if port <= 0 {
					t.Errorf("Expected system to assign a valid port number, but got %d", port)
				}

				// Call the actual test-defined command function
				return cmdFunc(dest, port)
			}

			// Run the command
			err := tunnel.Run("", cmdWithAssertions)

			// Verify expected error
			if !errors.Is(err, tt.expectedErr) {
				t.Errorf("Run() error = %v, want %v", err, tt.expectedErr)
			}
		})
	}
}

// TestRunWithKeepAlive tests that RunWithKeepAlive calls KeepAlive and then Run
func TestRunWithKeepAlive(t *testing.T) {
	ctx := context.Background()

	keepAliveCalled := false

	// Create a mock port forwarder
	mockPortForwarder := &PortForwarderMock{
		ForwardPortToListenerFunc: func(ctx context.Context, opts portforwarder.ForwardPortOpts, listener *net.TCPListener) error {
			return nil
		},
		CloseFunc: func() error {
			return nil
		},
	}

	// Create a mock invoker that tracks when KeepAlive is called
	mockInvoker := &InvokerMock{
		KeepAliveFunc: func() {
			keepAliveCalled = true
		},
		CloseFunc: func() error {
			return nil
		},
	}

	tunnel := &Tunnel{
		fwd: mockPortForwarder,
		invoker: &sshInvoker{
			port:    0, // Port will be assigned by Forward
			user:    "testuser",
			Invoker: mockInvoker,
		},
		closed: make(chan error, 1),
	}

	// Use Forward method to set up the tunnel and get a random port assigned
	if err := tunnel.Forward(ctx, 0); err != nil {
		t.Fatalf("Failed to forward: %v", err)
	}

	// Simple command function that returns immediately
	cmdFunc := func(dest string, port int) error {
		// Verify parameters
		if dest != "testuser@localhost" {
			t.Errorf("Expected destination %q, got %q", "testuser@localhost", dest)
		}
		if port <= 0 {
			t.Errorf("Expected system to assign a valid port number, but got %d", port)
		}
		return nil
	}

	// Run the command with keep alive
	err := tunnel.RunWithKeepAlive("", cmdFunc)
	if err != nil {
		t.Errorf("RunWithKeepAlive() error = %v", err)
	}

	// Verify that KeepAlive was called
	if !keepAliveCalled {
		t.Error("KeepAlive was not called")
	}
}
