package main

import (
	"gtorrent/config"
	"gtorrent/db"
	"gtorrent/torrent"
	"os"

	"github.com/alecthomas/kong"
	"github.com/rs/zerolog/log"
)

const VERSION = "0.1.0"

var CLI struct {
	Verify struct {
		Torrent     string `arg:"" help:"Torrent file to verify." type:"existingfile"`
		ContentPath string `arg:"" optional:"" help:"Path to the content files." type:"existingdir"`
	} `cmd:"" help:"Verify a torrent file."`
	Download struct {
		Torrent string `arg:"" help:"Torrent file to download."`
	} `cmd:"" help:"Download a torrent file."`
}
var mainDB *db.Database

func main() {
	println("goTorrent v" + VERSION)
	initConfig()
	initLogging()
	defer shutdownLogging()
	ctx := kong.Parse(&CLI)
	cmd := ctx.Command()
	switch cmd {
	case "verify <torrent> <content-path>":
		err := torrent.VerifyTorrent(CLI.Verify.Torrent, CLI.Verify.ContentPath)
		if err != nil {
			log.Error().Err(err).Msg("Error verifying torrent")
			return
		}
		println("Torrent verified successfully.")
	case "download <torrent>":
		initDB()
		err := DownloadTorrent(CLI.Download.Torrent)
		if err != nil {
			log.Error().Err(err).Msg("Error downloading torrent")
			return
		}
	default:
		ctx.PrintUsage(false)
	}

}

func initConfig() {
	// create the cache directory
	if err := os.MkdirAll(config.Main.CacheDir, os.ModePerm); err != nil {
		log.Fatal().Err(err).Str("path", config.Main.CacheDir).Msg("Failed to create cache directory")
	}

	// create the download directory
	if err := os.MkdirAll(config.Main.DownloadDir, os.ModePerm); err != nil {
		log.Fatal().Err(err).Str("path", config.Main.DownloadDir).Msg("Failed to create download directory")
	}
}

func initDB() {
	var err error
	mainDB, err = db.Init()
	if err != nil {
		log.Fatal().Err(err).Msg("Error initializing database")
	}
}
