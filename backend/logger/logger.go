package logger

import (
	"fmt"
	"io"
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
//
// The file is optional. With LOG_TO_STDOUT=true, or when the file cannot be
// created, the logger writes to stdout alone instead of failing. The project
// root comes from the source path baked in at compile time, which only exists
// where the binary was built: on a host that ships just the binary (Railway's
// Railpack image) the lookup fails, and a missing log folder must not be what
// stops the server booting. Hosts like that read stdout anyway.
func New(relativePath string, prefix string) (*LogCloser, error) {
	if utils.TruthyEnv("LOG_TO_STDOUT") {
		return stdoutLogger(prefix), nil
	}

	logCloser, err := newFileLogger(relativePath, prefix)
	if err != nil {
		logCloser = stdoutLogger(prefix)
		logCloser.Logf("log file %s unavailable (%s); logging to stdout only", relativePath, err)
	}
	return logCloser, nil
}

func stdoutLogger(prefix string) *LogCloser {
	return &LogCloser{logger: log.New(os.Stdout, prefix, log.Ldate|log.Ltime|log.Llongfile)}
}

func newFileLogger(relativePath string, prefix string) (*LogCloser, error) {
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

	// Written to stderr as well as to the file.
	//
	// These two streams were separate, and the split cost real time twice. The
	// request log goes to stdout, which whoever started the process is
	// capturing; everything written through here went only to a file under
	// logs/prod — a name that reads as "not this environment" when you are
	// debugging a local run. So the one place people look held the 500s and not
	// the reasons for them.
	//
	// It has caught out enough people to be written down: TESTING-LOG.md says
	// plainly that the real errors live in backend/logs/prod/app.log and not in
	// the stdout log. A note telling people where the other half is, is a
	// weaker fix than putting both halves in the same place.
	//
	// The file stays, because it survives whatever is or is not capturing
	// stderr, and because things already read it.
	logger := log.New(io.MultiWriter(logFile, os.Stderr), prefix, log.Ldate|log.Ltime|log.Llongfile)

	return &LogCloser{logger: logger, file: logFile}, nil
}

func (l *LogCloser) Logf(message string, a ...any) {
	formatted := fmt.Sprintf(message, a...)
	l.logger.Printf("\n	%s\n", formatted)
}

func (l *LogCloser) Close() error {
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}
