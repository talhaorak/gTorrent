package config

import "os"

type DBConfig struct {
	Path string
}

func NewDBConfig() *DBConfig {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "storage/state.db"
	}
	return &DBConfig{
		Path: path,
	}
}
