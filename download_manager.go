package main

import (
	"crypto/sha1"
	"fmt"
	"gtorrent/db/models"
	"gtorrent/torrent"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// startDownloadFromPeers initiates the download process from the discovered peers.
// It coordinates downloading pieces from multiple peers in parallel and handles
// piece verification, reassembly, and error recovery.
// Parameters:
//   - tor: Torrent metadata
//   - peers: Map of discovered peers
//   - downloadPath: Path where downloaded content will be saved
//   - dlModel: Database model for tracking download progress
//
// Returns an error if the download process fails.
func startDownloadFromPeers(tor *torrent.Torrent, peers map[string]*torrent.Peer, downloadPath string, dlModel *models.Download) error {
	// Create files with zero bytes
	err := createEmptyFiles(tor, downloadPath)
	if err != nil {
		return fmt.Errorf("failed to create files: %w", err)
	}

	totalPieces := len(tor.Pieces)
	if totalPieces == 0 {
		return fmt.Errorf("no pieces found in torrent")
	}

	// Create a bitfield to track downloaded pieces
	downloaded := make([]bool, totalPieces)
	var downloadMutex sync.Mutex

	// Create a channel to coordinate worker goroutines
	pieceQueue := make(chan int, totalPieces)
	// Fill the queue with piece indices
	for i := 0; i < totalPieces; i++ {
		pieceQueue <- i
	}

	log.Info().Msgf("Starting download of %d pieces with %d peers", totalPieces, len(peers))

	// Create worker pool based on available peers (max 5 connections per peer)
	maxWorkers := len(peers) * 5
	if maxWorkers > 20 {
		maxWorkers = 20 // Cap at 20 concurrent downloads
	}
	if maxWorkers < 5 {
		maxWorkers = 5 // At least 5 concurrent downloads
	}

	var wg sync.WaitGroup
	errChan := make(chan error, maxWorkers)
	doneChan := make(chan bool)

	// Progress reporting goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				downloadMutex.Lock()
				completedPieces := 0
				for _, isDownloaded := range downloaded {
					if isDownloaded {
						completedPieces++
					}
				}
				progress := float64(completedPieces) / float64(totalPieces) * 100.0
				downloadMutex.Unlock()

				// Update progress in database
				dlModel.Progress = int(progress)
				mainDB.UpdateDownload(dlModel)

				log.Info().Msgf("Download progress: %.2f%% (%d/%d pieces)",
					progress, completedPieces, totalPieces)
			case <-doneChan:
				return
			}
		}
	}()

	// Start worker goroutines
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for pieceIndex := range pieceQueue {
				// Check if this piece is already downloaded
				downloadMutex.Lock()
				if downloaded[pieceIndex] {
					downloadMutex.Unlock()
					continue
				}
				downloadMutex.Unlock()

				// Try to download piece from available peers
				piece, err := downloadPieceFromPeers(tor, pieceIndex, peers)
				if err != nil {
					errChan <- fmt.Errorf("worker %d failed to download piece %d: %w",
						workerID, pieceIndex, err)
					// Put the piece back in the queue for retry
					pieceQueue <- pieceIndex
					continue
				}

				// Verify the piece hash
				expectedHash := tor.Pieces[pieceIndex]
				hash := sha1.Sum(piece)
				actualHash := fmt.Sprintf("%x", hash)

				if actualHash != expectedHash {
					log.Warn().Msgf("Piece %d hash mismatch, retrying", pieceIndex)
					// Put the piece back in the queue for retry
					pieceQueue <- pieceIndex
					continue
				}

				// Write the piece to the correct file(s)
				err = writePiece(tor, pieceIndex, piece, downloadPath)
				if err != nil {
					errChan <- fmt.Errorf("worker %d failed to write piece %d: %w",
						workerID, pieceIndex, err)
					// Put the piece back in the queue for retry
					pieceQueue <- pieceIndex
					continue
				}

				// Mark the piece as downloaded
				downloadMutex.Lock()
				downloaded[pieceIndex] = true
				completedPieces := 0
				for _, isDownloaded := range downloaded {
					if isDownloaded {
						completedPieces++
					}
				}

				// Check if download is complete
				if completedPieces == totalPieces {
					close(pieceQueue) // Signal other workers to stop
				}
				downloadMutex.Unlock()
			}
		}(i)
	}

	// Wait for all workers to finish or for an error
	go func() {
		wg.Wait()
		close(doneChan)
		close(errChan)
	}()

	// Handle and aggregate errors
	for err := range errChan {
		log.Error().Err(err).Msg("Error during download")
		// Continue downloading despite errors - we'll retry pieces
	}

	// Check if all pieces were downloaded successfully
	downloadMutex.Lock()
	allDownloaded := true
	for _, isDownloaded := range downloaded {
		if !isDownloaded {
			allDownloaded = false
			break
		}
	}
	downloadMutex.Unlock()

	if !allDownloaded {
		return fmt.Errorf("download incomplete - some pieces could not be downloaded")
	}

	// Download completed successfully
	dlModel.Status = models.DownloadComplete
	dlModel.Progress = 100
	dlModel.CompletedAt = time.Now().Unix()
	mainDB.UpdateDownload(dlModel)

	log.Info().Msg("Download completed successfully")
	return nil
}

// createEmptyFiles creates empty files with the correct sizes as specified in the torrent.
// This pre-allocates the space needed for the download.
func createEmptyFiles(tor *torrent.Torrent, downloadPath string) error {
	for _, file := range tor.FileList {
		filePath := filepath.Join(downloadPath, file.Path)

		// Create directory structure if needed
		err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm)
		if err != nil {
			return err
		}

		// Create empty file with correct size
		f, err := os.Create(filePath)
		if err != nil {
			return err
		}

		// Pre-allocate space
		err = f.Truncate(file.Length)
		f.Close() // Close file regardless of error
		if err != nil {
			return err
		}
	}
	return nil
}

// downloadPieceFromPeers attempts to download a specific piece from available peers.
// It tries different peers until the piece is successfully downloaded.
func downloadPieceFromPeers(tor *torrent.Torrent, pieceIndex int, peers map[string]*torrent.Peer) ([]byte, error) {
	// This is a placeholder implementation that would be replaced with actual peer communication
	// In a real implementation, this would:
	// 1. Send request messages to peers
	// 2. Receive piece data
	// 3. Handle timeouts and connection issues

	// For now, return a simulated piece for demonstration purposes
	// This needs to be replaced with actual peer communication code
	log.Debug().Msgf("Would download piece %d from peers", pieceIndex)

	// Placeholder: Simulate a successful download with random data
	// In a real implementation, remove this and implement proper peer protocol
	pieceLength := tor.PieceLength
	if pieceIndex == len(tor.Pieces)-1 {
		// Last piece might be shorter
		lastPieceSize := tor.Length % tor.PieceLength
		if lastPieceSize > 0 {
			pieceLength = lastPieceSize
		}
	}

	// This is just a placeholder and should be replaced with actual peer communication
	return make([]byte, pieceLength), nil
}

// writePiece writes a downloaded piece to the correct position in the file(s).
// A single piece may span multiple files in a multi-file torrent.
func writePiece(tor *torrent.Torrent, pieceIndex int, pieceData []byte, downloadPath string) error {
	pieceOffset := int64(pieceIndex) * tor.PieceLength
	pieceLength := int64(len(pieceData))

	// Find the file(s) this piece belongs to
	var currentOffset int64 = 0
	for _, file := range tor.FileList {
		filePath := filepath.Join(downloadPath, file.Path)

		fileStart := currentOffset
		fileEnd := currentOffset + file.Length

		// Check if this piece overlaps with the current file
		if pieceOffset < fileEnd && pieceOffset+pieceLength > fileStart {
			// Calculate the overlap
			pieceStartInFile := int64(0)
			if pieceOffset > fileStart {
				pieceStartInFile = pieceOffset - fileStart
			}

			fileStartInPiece := int64(0)
			if fileStart > pieceOffset {
				fileStartInPiece = fileStart - pieceOffset
			}

			bytesToWrite := pieceLength - fileStartInPiece
			if fileEnd < pieceOffset+pieceLength {
				bytesToWrite = fileEnd - (pieceOffset + fileStartInPiece)
			}

			// Open the file for writing
			f, err := os.OpenFile(filePath, os.O_WRONLY, 0644)
			if err != nil {
				return err
			}

			// Seek to the correct position
			_, err = f.Seek(pieceStartInFile, io.SeekStart)
			if err != nil {
				f.Close()
				return err
			}

			// Write the piece data
			_, err = f.Write(pieceData[fileStartInPiece : fileStartInPiece+bytesToWrite])
			f.Close() // Close file regardless of error
			if err != nil {
				return err
			}
		}

		currentOffset += file.Length
	}

	return nil
}
