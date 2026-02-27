package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	file    *os.File
	lgr     *log.Logger
	enabled bool
	start   time.Time
)

// Init opens the log file and enables debug logging.
// If path is empty, logging is disabled.
func Init(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	mu.Lock()
	file = f
	lgr = log.New(f, "", 0)
	enabled = true
	start = time.Now()
	mu.Unlock()
	Log("logger", "initialized, writing to %s", path)
	return nil
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
		enabled = false
	}
}

// Log writes a timestamped debug line. The component identifies the subsystem
// (e.g. "ssh", "ui", "app"). Safe to call from any goroutine.
func Log(component, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	if !enabled {
		return
	}
	elapsed := time.Since(start)
	msg := fmt.Sprintf(format, args...)
	lgr.Printf("[%10s] [%-10s] %s", elapsed.Truncate(time.Millisecond), component, msg)
}
