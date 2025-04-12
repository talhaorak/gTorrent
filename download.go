package main

import (
	"fmt"
	"gtorrent/config"
	"gtorrent/db/models"
	"gtorrent/torrent"
	"gtorrent/utils"
	"path/filepath"
	"time"

	"os"
	"sync"

	"github.com/rs/zerolog/log"
)

// DownloadTorrent initiates the download of content defined in a torrent file.
// It reads the torrent file, parses its contents, copies it to the cache directory,
// creates a database entry for the download, and contacts trackers to find peers.
// Parameters:
//   - torrentFile: Path to the .torrent file to be downloaded
//
// Returns an error if any step of the process fails, or nil on success.
func DownloadTorrent(torrentFile string) error {
	log.Info().Msg("Downloading torrent: " + torrentFile)

	content, err := os.ReadFile(torrentFile)
	if err != nil {
		return err
	}
	tor, err := torrent.TorrentFromBytes(content)
	if err != nil {
		return err
	}

	// copy the torrent file into cacheDir
	torrentFilename := filepath.Base(torrentFile)

	// write the torrent file to the cacheDir
	cachePath := filepath.Join(config.Main.CacheDir, torrentFilename)
	err = utils.CopyFile(torrentFile, cachePath)
	if err != nil {
		return err
	}

	// check the mainDB for the torrent, if not found, add it
	dlModel, err := mainDB.CreateDownload(tor, cachePath)
	if err != nil {
		return err
	}

	trackers := make([]torrent.ITracker, 0)
	for _, announce := range tor.AnnounceList {
		tracker, err := torrent.NewTracker(announce)
		if err != nil {
			log.Warn().Err(err).Str("tracker", announce).Msg("Failed to create tracker, skipping")
			continue
		}
		trackers = append(trackers, tracker)
	}

	// Only fail if we have no working trackers
	if len(trackers) == 0 {
		return fmt.Errorf("no valid trackers found")
	}

	// Get the peers from the trackers
	me := torrent.PeerMe()
	peers := make(map[string]*torrent.Peer)

	wg := sync.WaitGroup{}
	for trackerIndex, tracker := range trackers {
		wg.Add(1)
		go func(trIndex int, tr torrent.ITracker) {
			defer wg.Done()
			log.Info().Msg("Getting peers from tracker: " + tr.Announce())
			tPeers, err := tr.GetPeers(tor, me)
			trackerModel := &dlModel.Trackers[trIndex]
			if err != nil {
				log.Error().Err(err).Msg("Error getting peers from tracker")
				trackerModel.Status = models.TrackerError
				trackerModel.LastError = err.Error()
				mainDB.UpdateTracker(trackerModel)
				return
			}
			log.Info().Msgf("Got %d peers from tracker", len(tPeers))
			trackerModel.Status = models.TrackerComplete
			trackerModel.Seeders = tr.Seeders()
			trackerModel.Leechers = tr.Leechers()

			for _, peer := range tPeers {
				if peer.String() == me.String() {
					continue
				}
				if peer.IP == "0.0.0.0" {
					continue
				}

				_, ok := peers[peer.String()]
				if !ok {
					peers[peer.String()] = peer
					mainDB.CreatePeer(trackerModel, peer)
				}
			}

			trackerModel.LastCheck = time.Now().Unix()
			mainDB.UpdateTracker(trackerModel)
		}(trackerIndex, tracker)
	}
	wg.Wait()

	// Update the download status
	dlModel.Status = models.DownloadInProgress
	mainDB.UpdateDownload(dlModel)

	log.Info().Msgf("Found %d peers for download", len(peers))
	if len(peers) == 0 {
		log.Warn().Msg("No peers found for download, will retry later")
		return nil
	}

	// Create destination directory
	downloadPath := filepath.Join(config.Main.DownloadDir, tor.Name)
	err = os.MkdirAll(downloadPath, os.ModePerm)
	if err != nil {
		dlModel.Status = models.DownloadError
		dlModel.LastError = fmt.Sprintf("Failed to create download directory: %s", err.Error())
		mainDB.UpdateDownload(dlModel)
		return err
	}

	// Initialize download manager and start download
	log.Info().Msg("Starting download of pieces")
	err = startDownloadFromPeers(tor, peers, downloadPath, dlModel)
	if err != nil {
		dlModel.Status = models.DownloadError
		dlModel.LastError = err.Error()
		mainDB.UpdateDownload(dlModel)
		return err
	}

	return nil
}
