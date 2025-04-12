package main

import (
	"os"

	"github.com/rs/zerolog/log"

	"github.com/rs/zerolog"
)

const logFilename = "gorrent.log"

var logFile *os.File

func initLogging() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	var err error
	logFile, err = os.OpenFile(logFilename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		println("Error opening log file: " + err.Error())
	}
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	logger := zerolog.New(multi).With().Timestamp().Logger()
	log.Logger = logger

	log.Info().Msgf("gorrent v%s", VERSION)
}

func shutdownLogging() {
	logFile.Close()
}
