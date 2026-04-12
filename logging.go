package main

import (
	"log"
	"os"
	"sync/atomic"
)

var (
	logger       *log.Logger
	debugEnabled atomic.Bool
)

func initLogger() *os.File {
	f, err := os.OpenFile("incott.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)
		return nil
	}
	logger = log.New(f, "", log.Ldate|log.Ltime)
	return f
}

// logInfo writes a message that is always recorded regardless of the debug toggle.
// Use for user-initiated actions: setting changes, auto-boost events, startup/shutdown.
func logInfo(format string, args ...any) {
	if logger == nil {
		return
	}
	if len(args) > 0 {
		logger.Printf("[INFO] "+format, args...)
	} else {
		logger.Println("[INFO] " + format)
	}
}

// logDebug writes a message only when the user has enabled debug mode.
// Use for device-level events: HID report bytes, poll responses, raw data.
func logDebug(format string, args ...any) {
	if !debugEnabled.Load() || logger == nil {
		return
	}
	if len(args) > 0 {
		logger.Printf("[DEBUG] "+format, args...)
	} else {
		logger.Println("[DEBUG] " + format)
	}
}
