package db

import (
	"gtorrent/config"
	"gtorrent/db/models"
	"gtorrent/torrent"
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Database struct {
	db *gorm.DB
}

func Init() (*Database, error) {
	db, err := gorm.Open(sqlite.Open(config.Main.DB.Path), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	err = db.AutoMigrate(&models.Download{}, &models.Peer{}, &models.Piece{}, &models.Tracker{})
	if err != nil {
		log.Fatal(err)
	}

	return &Database{
		db: db,
	}, nil
}

func (d *Database) Close() {
	sqlDB, err := d.db.DB()
	if err != nil {
		log.Fatal(err)
	}
	sqlDB.Close()
}

func (d *Database) CreateDownload(tor *torrent.Torrent, torrentPath string) (*models.Download, error) {
	// check if the download already exists, by checking the infohash
	// if it does, return the download
	// if it doesn't, create a new download and return it
	download := &models.Download{}
	var err error
	tx := d.db.Where("info_hash = ?", tor.InfoHashString()).First(download)
	if tx.Error == nil {
		goto fillup
	}

	download = &models.Download{
		InfoHash:        tor.InfoHashString(),
		Name:            tor.Name,
		TorrentFilename: torrentPath,
		Status:          models.Downloading,
		DownloadDir:     config.Main.DownloadDir,
		TotalSize:       tor.Length,
	}

	// for i, pieceHash := range tor.Pieces {
	// 	download.Pieces[i] = models.Piece{
	// 		Hash:         pieceHash,
	// 		IsDownloaded: false,
	// 	}
	// }

	// for i, announce := range tor.AnnounceList {
	// 	download.Trackers[i] = models.Tracker{
	// 		Announce: announce,
	// 		Status:   models.TrackerAnnouncing,
	// 	}
	// }

	err = d.db.Create(download).Error
	if err != nil {
		return nil, err
	}

	for _, pieceHash := range tor.Pieces {
		piece := &models.Piece{
			DownloadID: download.ID,
			Hash:       pieceHash,
		}
		err = d.db.Create(piece).Error
		if err != nil {
			return nil, err
		}
	}

	for _, announce := range tor.AnnounceList {
		tracker := &models.Tracker{
			DownloadID: download.ID,
			Announce:   announce,
			Status:     models.TrackerAnnouncing,
		}
		err = d.db.Create(tracker).Error
		if err != nil {
			return nil, err
		}
	}

	// for _, pieceHash := range tor.Pieces {
	// 	piece := &PieceModel{
	// 		Download:     download,
	// 		Hash:         pieceHash,
	// 		IsDownloaded: false,
	// 	}
	// 	err = d.db.Create(piece).Error
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	// for _, announce := range tor.AnnounceList {
	// 	tracker := &TrackerModel{
	// 		Download: download,
	// 		Announce: announce,
	// 	}
	// 	err = d.db.Create(tracker).Error
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }
fillup:
	result := d.db.Preload("Trackers").Preload("Pieces").First(download)
	if result.Error != nil {
		return nil, result.Error
	}
	return download, nil
}

func (d *Database) UpdateTracker(tracker *models.Tracker) error {
	return d.db.Save(tracker).Error
}

func (d *Database) CreatePeers(tracker *models.Tracker, peers []*torrent.Peer) error {
	for _, peer := range peers {
		err := d.CreatePeer(tracker, peer)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) CreatePeer(tracker *models.Tracker, peer *torrent.Peer) error {
	newPeer := &models.Peer{
		DownloadID: tracker.DownloadID,
		TrackerID:  tracker.ID,
		IP:         peer.IP,
		Port:       peer.Port,
		IsStopped:  true,
	}
	// if a peer with the same trackerID, IP and Port already exists, update it, otherwise create a new one
	existingPeer := &models.Peer{}
	result := d.db.Where("download_id = ? AND ip = ? AND port = ?", tracker.ID, peer.IP, peer.Port).First(existingPeer)
	if result.Error == nil {
		newPeer.ID = existingPeer.ID
		result = d.db.Save(newPeer)
		return result.Error
	} else {
		result = d.db.Create(newPeer)
		return result.Error
	}
}
