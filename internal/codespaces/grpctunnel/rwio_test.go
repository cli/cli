package grpctunnel

import (
	"errors"
	"io"
	"testing"
)

// mockReadCloser implements io.ReadCloser for testing
type mockReadCloser struct {
	closeErr error
	readErr  error
	closed   bool
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	return 0, io.EOF
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return m.closeErr
}

// mockWriteCloser implements io.WriteCloser for testing
type mockWriteCloser struct {
	closeErr error
	writeErr error
	closed   bool
}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	m.closed = true
	return m.closeErr
}

func TestRwioClose(t *testing.T) {
	tests := []struct {
		name          string
		readCloseErr  error
		writeCloseErr error
		wantErr       bool
		expectedErr   error
	}{
		{
			name:          "no errors",
			readCloseErr:  nil,
			writeCloseErr: nil,
			wantErr:       false,
		},
		{
			name:          "read error only",
			readCloseErr:  errors.New("read close error"),
			writeCloseErr: nil,
			wantErr:       true,
			expectedErr:   errors.New("read close error"),
		},
		{
			name:          "write error only",
			readCloseErr:  nil,
			writeCloseErr: errors.New("write close error"),
			wantErr:       true,
			expectedErr:   errors.New("write close error"),
		},
		{
			name:          "both errors (write takes precedence)",
			readCloseErr:  errors.New("read close error"),
			writeCloseErr: errors.New("write close error"),
			wantErr:       true,
			expectedErr:   errors.New("write close error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockReader := &mockReadCloser{closeErr: tt.readCloseErr}
			mockWriter := &mockWriteCloser{closeErr: tt.writeCloseErr}

			r := &rwio{
				ReadCloser:  mockReader,
				WriteCloser: mockWriter,
			}

			err := r.Close()

			// Check if both underlying Close methods were called
			if !mockReader.closed {
				t.Error("ReadCloser.Close was not called")
			}
			if !mockWriter.closed {
				t.Error("WriteCloser.Close was not called")
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				if err.Error() != tt.expectedErr.Error() {
					t.Errorf("Error message = %v, want %v", err, tt.expectedErr)
				}
			}
		})
	}
}

func TestRwioCloseWrite(t *testing.T) {
	tests := []struct {
		name          string
		writeCloseErr error
		wantErr       bool
	}{
		{
			name:          "no error",
			writeCloseErr: nil,
			wantErr:       false,
		},
		{
			name:          "with error",
			writeCloseErr: errors.New("write close error"),
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockWriter := &mockWriteCloser{closeErr: tt.writeCloseErr}

			r := &rwio{
				ReadCloser:  &mockReadCloser{},
				WriteCloser: mockWriter,
			}

			err := r.CloseWrite()

			// Check if WriteCloser.Close was called
			if !mockWriter.closed {
				t.Error("WriteCloser.Close was not called")
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("CloseWrite() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
