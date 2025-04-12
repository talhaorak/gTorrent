package models

import "gorm.io/gorm"

type Download struct {
	gorm.Model
	InfoHash        string `gorm:"uniqueIndex"`
	Name            string
	TorrentFilename string
	Status          DownloadStatus
	DownloadDir     string
	TotalSize       int64
	DownloadedSize  int64

	Peers    []Peer
	Pieces   []Piece
	Trackers []Tracker
}

type DownloadStatus = string

const (
	Invalid     DownloadStatus = "invalid"
	Downloading DownloadStatus = "downloading"
	Complete    DownloadStatus = "complete"
	Error       DownloadStatus = "error"
	Paused      DownloadStatus = "paused"
)

type Peer struct {
	ID           uint `gorm:"primaryKey"`
	DownloadID   uint
	TrackerID    uint `gorm:"foreignKey:Trackers"`
	IP           string
	Port         uint16
	IsSeeder     bool
	IsStopped    bool
	IsChoked     bool
	IsInterested bool
}

type Piece struct {
	ID           uint `gorm:"primaryKey"`
	DownloadID   uint
	Index        int
	Hash         string
	IsDownloaded bool
}

type Tracker struct {
	ID         uint `gorm:"primaryKey"`
	DownloadID uint
	Announce   string
	Status     TrackerStatus
	LastCheck  int64
	LastError  string
	NextCheck  int64
	// for http tracker
	Interval    int
	MinInterval int
	Seeders     int
	Leechers    int

	// for udp tracker
	ConnectionID  int64
	TransactionID int
}

type TrackerStatus = string

const (
	TrackerInvalid    TrackerStatus = "invalid"
	TrackerAnnouncing TrackerStatus = "announcing"
	TrackerError      TrackerStatus = "error"
	TrackerComplete   TrackerStatus = "complete"
)
