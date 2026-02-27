package service

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WriteStartupErrorFile writes a startup error to a file so that it is
// visible even when the logger has not been initialized and the Windows
// Event Log is hard to find. The file is overwritten on each call so
// that only the most recent error is kept.
func WriteStartupErrorFile(logDir string, err error) {
	_ = os.MkdirAll(logDir, 0755)

	path := filepath.Join(logDir, "startup-error.log")
	f, ferr := os.Create(path)
	if ferr != nil {
		return
	}
	defer f.Close()

	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "[%s] STARTUP ERROR\n%v\n", ts, err)
}
