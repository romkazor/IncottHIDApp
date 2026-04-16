package main

import (
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed icons/tray_icon.ico
var iconData []byte

// version is overridden at build time via -ldflags="-X main.version=v0.1.0"
var version = "dev"

func main() {
	if !acquireSingleInstance() {
		showAlreadyRunningDialog()
		return
	}
	initPaths()
	loadConfig()

	logFile := initLogger()
	if logFile != nil {
		defer logFile.Close()
	}

	logInfo("=== IncottHIDApp started (version: %s) ===", version)

	setAutoStart(autoStartEnabled)

	go mouseWorker()
	go gameMonitorWorker()
	go updateCheckWorker()
	systray.Run(onReady, onExit)
}

func onExit() {}
