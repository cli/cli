// Package cliutil contains I/O shared by the skill's local helper commands.
package cliutil

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// NewRequestID creates a private transport identifier with the established hexadecimal shape.
func NewRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

// ManifestHash binds the exact pinned Node manifest bytes used by the adapter.
func ManifestHash(skillRoot string) (string, error) {
	hash := sha256.New()
	for _, name := range []string{"package.json", "package-lock.json"} {
		data, err := os.ReadFile(filepath.Join(skillRoot, "scripts", name))
		if err != nil {
			return "", err
		}
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(data)))
		hash.Write(size[:])
		hash.Write(data)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Streams keeps command input, data output, and diagnostics independently testable.
type Streams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer
}

// ReadJSON reads one bounded regular file and preserves its exact source bytes.
func ReadJSON(filename string, limit int64, target any) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open JSON document: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect JSON document: %w", err)
	}
	if !info.Mode().IsRegular() || limit <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("JSON document must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read JSON document: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("JSON document exceeds its size limit")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return nil, fmt.Errorf("decode JSON document: %w", err)
	}
	return data, nil
}

// SHA256File fingerprints an existing regular file without loading it all into memory.
func SHA256File(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("fingerprint input must be a regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// WriteJSON atomically writes private JSON into an already prepared directory.
func WriteJSON(filename string, value any) (err error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	pending, err := os.CreateTemp(filepath.Dir(filename), ".cli-exercise-*.pending")
	if err != nil {
		return err
	}
	name := pending.Name()
	defer func() {
		if pending != nil {
			if closeErr := pending.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		if err != nil {
			if removeErr := os.Remove(name); removeErr != nil && !os.IsNotExist(removeErr) {
				err = fmt.Errorf("%w; remove temporary JSON: %v", err, removeErr)
			}
		}
	}()
	if _, err = pending.Write(append(data, '\n')); err != nil {
		return err
	}
	if err = pending.Sync(); err != nil {
		return err
	}
	err = pending.Close()
	pending = nil
	if err != nil {
		return err
	}
	if err = os.Rename(name, filename); err != nil {
		return err
	}
	return nil
}

// JSON writes one structured response without mixing diagnostics into it.
func JSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

// Error writes the shared structured diagnostic envelope.
func Error(writer io.Writer, code, message string) error {
	return JSON(writer, struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{
		Error: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: message},
	})
}
