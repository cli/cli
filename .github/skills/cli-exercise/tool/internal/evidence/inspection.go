package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"os"
	"path/filepath"
)

type renderedOccurrence struct {
	Image              string   `json:"image"`
	FirstFrame         int      `json:"firstFrame"`
	LastFrame          int      `json:"lastFrame"`
	CaptureTimeSeconds float64  `json:"captureTimeSeconds"`
	SourceState        int      `json:"sourceState"`
	Revision           int      `json:"revision"`
	NoteTimeSeconds    *float64 `json:"noteTimeSeconds,omitempty"`
}

type sourceOccurrence struct {
	Image           string  `json:"image"`
	State           int     `json:"state"`
	Time            float64 `json:"t"`
	Revision        int     `json:"revision"`
	AlternateScreen bool    `json:"alternateScreen"`
}

type imageStore struct {
	directory string
	images    map[[32]byte]string
}

func newImageStore(inspection string) (*imageStore, error) {
	directory := filepath.Join(inspection, "images")
	if err := os.Mkdir(directory, 0o700); err != nil {
		return nil, err
	}
	return &imageStore{directory: directory, images: map[[32]byte]string{}}, nil
}

func pixelHash(canvas *image.RGBA) [32]byte {
	hash := sha256.New()
	var dimensions [16]byte
	binary.BigEndian.PutUint64(dimensions[:8], uint64(canvas.Bounds().Dx()))
	binary.BigEndian.PutUint64(dimensions[8:], uint64(canvas.Bounds().Dy()))
	hash.Write(dimensions[:])
	for y := canvas.Bounds().Min.Y; y < canvas.Bounds().Max.Y; y++ {
		start := canvas.PixOffset(canvas.Bounds().Min.X, y)
		hash.Write(canvas.Pix[start : start+canvas.Bounds().Dx()*4])
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result
}

func (store *imageStore) save(canvas *image.RGBA) (string, error) {
	hash := pixelHash(canvas)
	if path, exists := store.images[hash]; exists {
		return path, nil
	}
	name := fmt.Sprintf("image-%06d.png", len(store.images))
	if err := savePNG(filepath.Join(store.directory, name), canvas); err != nil {
		return "", err
	}
	path := filepath.ToSlash(filepath.Join("images", name))
	store.images[hash] = path
	return path, nil
}
