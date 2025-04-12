package main

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/rs/zerolog"
)

var logFile *os.File

func initLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}

	// Get log file path from environment or use default
	logFilePath := os.Getenv("LOG_FILE")
	if logFilePath == "" {
		logFilePath = "gorrent.log"
	}

	// Ensure the directory exists for the log file if it contains a path
	logDir := filepath.Dir(logFilePath)
	if logDir != "." {
		if err := os.MkdirAll(logDir, os.ModePerm); err != nil {
			println("Error creating log directory: " + err.Error())
		}
	}

	var err error
	logFile, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		println("Error opening log file: " + err.Error())
	}
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	logger := zerolog.New(multi).With().Timestamp().Logger()
	log.Logger = logger

	log.Info().Msgf("gorrent v%s", VERSION)
}

// shutdownLogging safely closes the log file if it's open.
// This should be called when the application is shutting down.
func shutdownLogging() {
	if logFile != nil {
		err := logFile.Close()
		if err != nil {
			println("Error closing log file: " + err.Error())
		}
	}
}
