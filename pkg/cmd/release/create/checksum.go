package create

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cli/cli/pkg/cmd/release/shared"
)

func createChecksumAssetFor(assets []*shared.AssetForUpload) (shared.AssetForUpload, error) {
	checksumData, err := generateChecksumFromAssets(assets)

	if err != nil {
		return shared.AssetForUpload{}, err
	}
	err = writeToFile(checksumData)
	if err != nil {
		return shared.AssetForUpload{}, err
	}

	args, err := shared.AssetsFromArgs([]string{"temperoryAssetChecksumFile.txt"})

	if err != nil {
		return shared.AssetForUpload{}, err
	}
	args[0].Name = "checksum.txt"
	return *args[0], nil
}

func generateChecksumFromAssets(assets []*shared.AssetForUpload) (map[string]string, error) {
	checksumData := make(map[string]string)
	for _, asset := range assets {
		file, err := asset.Open()
		if err != nil {
			return make(map[string]string), err
		}
		checksum, err := generateChecksum(file)
		fmt.Println(checksum)
		checksumData[asset.Name] = checksum
	}
	return checksumData, nil
}

func generateChecksum(file io.Reader) (string, error) {
	hashFunc := sha256.New()
	_, err := io.Copy(hashFunc, file)
	if err != nil {
		return "", fmt.Errorf("Checksum creation failed: %w", err)
	}
	return hex.EncodeToString(hashFunc.Sum(nil)), nil
}

func writeToFile(checksumData map[string]string) error {
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(path, "temperoryAssetChecksumFile.txt"), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	for fileName, checksum := range checksumData {
		file.WriteString(checksum + "  " + fileName + "\n")
	}
	file.Close()
	return nil
}

func deleteChecksumFile() error {
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(path, "temperoryAssetChecksumFile.txt"))
	if err != nil {
		return err
	}
	return nil
}
