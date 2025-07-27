package logger

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/SFLuv/app/backend/utils"
)

type LogCloser struct {
	logger *log.Logger
	file   *os.File
}

// New dynamically finds the root directory of your project. Pass in a relative path as though ./ is your project's root.
func New(relativePath string, prefix string) (*LogCloser, error) {
	root, err := utils.GetProjectRoot()
	if err != nil {
		return nil, err
	}

	path := path.Join(root, relativePath)
	dirPath := filepath.Dir(path)
	if !utils.Exists(dirPath) {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return nil, err
		}
	}

	logFile, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	logger := log.New(logFile, prefix, log.Ldate|log.Ltime|log.Llongfile)

	return &LogCloser{logger: logger, file: logFile}, nil
}

func (l *LogCloser) Logf(message string, a ...any) {
	formatted := fmt.Sprintf(message, a...)
	l.logger.Printf("\n	%s\n", formatted)
}

func (l *LogCloser) Close() error {
	return l.file.Close()
}
