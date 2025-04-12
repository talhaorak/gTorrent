package config

import (
	"os"

	"github.com/joho/godotenv"
)

type AppConfig struct {
	CacheDir    string
	DownloadDir string
	DB          *DBConfig
}

func NewAppConfig() *AppConfig {
	cacheDir := os.Getenv("CACHE_DIR")
	if cacheDir == "" {
		cacheDir = "storage/cache"
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if downloadDir == "" {
		downloadDir = "storage/downloads"
	}

	dbConf := NewDBConfig()

	return &AppConfig{
		CacheDir:    cacheDir,
		DownloadDir: downloadDir,
		DB:          dbConf,
	}
}

var Main *AppConfig

func init() {
	_ = godotenv.Load()
	Main = NewAppConfig()
}
