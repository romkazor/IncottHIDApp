package main

import (
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed icons/tray_icon.ico
var iconData []byte

func main() {
	loadConfig()

	logFile := initLogger()
	if logFile != nil {
		defer logFile.Close()
	}

	logInfo("=== Incott driver started ===")

	setAutoStart(autoStartEnabled)

	go mouseWorker()
	go gameMonitorWorker()
	systray.Run(onReady, onExit)
}

func onExit() {}
