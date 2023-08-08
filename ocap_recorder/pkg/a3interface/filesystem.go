package a3interface

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetArmaDir returns the Arma 3 executable directory. It will not account for symlinks.
func GetArmaDir() (string, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("Error getting executable directory: %s", err.Error())
	}
	dir := filepath.Dir(executablePath)
	return dir, nil
}
