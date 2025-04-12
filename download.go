package main

import (
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
			return err
		}
		trackers = append(trackers, tracker)
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
	return nil
}
