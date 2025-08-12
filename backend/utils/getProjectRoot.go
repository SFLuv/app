package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func GetProjectRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to get caller information")
	}
	currentDir := filepath.Dir(filename)

	for {
		// Check for common project root indicators
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			return currentDir, nil
		}
		if _, err := os.Stat(filepath.Join(currentDir, ".git")); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir { // Reached file system root
			return "", fmt.Errorf("project root not found")
		}
		currentDir = parentDir
	}
}
