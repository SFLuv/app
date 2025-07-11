package utils

import (
	"os"
)

func Exists(path string) bool {
	exists := true
	_, err := os.Stat(path)
	if err != nil {
		exists = false
	}

	if os.IsExist(err) {
		exists = true
	}

	return exists
}
