package main

import (
	"crypto/sha1"
	"fmt"
	"gtorrent/db/models"
	"gtorrent/torrent"
	"io"
	"net"
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

// peerConnectionState holds the state for a connection to a single peer
// during a piece download attempt.
type peerConnectionState struct {
	peer       *torrent.Peer
	conn       net.Conn
	bitfield   torrent.Bitfield
	peerChoked bool
	startTime  time.Time // To track connection duration/timeouts
}

// close closes the connection to the peer.
func (pcs *peerConnectionState) close() {
	if pcs.conn != nil {
		pcs.conn.Close()
	}
}

// sendRequest sends a Request message to the peer.
func (pcs *peerConnectionState) sendRequest(pieceIndex, begin, length uint32) error {
	reqPayload := torrent.FormatRequest(pieceIndex, begin, length)
	msg := torrent.Message{Type: torrent.MsgRequest, Payload: reqPayload}
	_, err := pcs.conn.Write(msg.Serialize())
	return err
}

// downloadPieceFromPeers attempts to download a specific piece from available peers.
// It tries different peers until the piece is successfully downloaded.
func downloadPieceFromPeers(tor *torrent.Torrent, pieceIndex int, peers map[string]*torrent.Peer) ([]byte, error) {
	pieceLength := tor.PieceLength
	if pieceIndex == len(tor.Pieces)-1 {
		lastPieceSize := tor.Length % tor.PieceLength
		if lastPieceSize > 0 {
			pieceLength = lastPieceSize
		}
	}

	// TODO: Get our actual Peer ID
	var selfPeerID [20]byte
	copy(selfPeerID[:], "-GT0001-000000000000") // Placeholder Peer ID

	// Iterate through available peers.
	for _, peer := range peers {
		state := &peerConnectionState{
			peer:       peer,
			peerChoked: true, // Assume choked initially
			startTime:  time.Now(),
		}

		log.Debug().Msgf("Attempting to download piece %d from peer %s", pieceIndex, peer.String())

		// 3. Establish connection
		conn, err := net.DialTimeout("tcp", peer.String(), 10*time.Second)
		if err != nil {
			log.Warn().Msgf("Failed to connect to peer %s: %v", peer.String(), err)
			continue // Try next peer
		}
		state.conn = conn
		defer state.close() // Ensure connection is closed

		// 4. Perform BitTorrent handshake
		_, err = torrent.PerformHandshake(state.conn, tor, selfPeerID)
		if err != nil {
			log.Warn().Msgf("Handshake failed with peer %s: %v", peer.String(), err)
			continue // Try next peer
		}
		log.Debug().Msgf("Handshake successful with peer %s", peer.String())

		// 5. Exchange messages (Bitfield, Interested, Unchoke)
		// Read the first message, expecting Bitfield (or Have)
		msg, err := readMessageWithTimeout(state.conn, 10*time.Second)
		if err != nil {
			log.Warn().Msgf("Failed to read initial message from peer %s: %v", peer.String(), err)
			continue
		}

		if msg.Type == torrent.MsgBitfield {
			if len(msg.Payload) != (len(tor.Pieces)+7)/8 {
				log.Warn().Msgf("Received invalid bitfield length from %s", peer.String())
				continue
			}
			state.bitfield = torrent.Bitfield(msg.Payload)
			log.Debug().Msgf("Received Bitfield from %s", peer.String())
		} else {
			// If no bitfield, initialize an empty one and process the first message (likely Have)
			state.bitfield = make(torrent.Bitfield, (len(tor.Pieces)+7)/8)
			if err := handleMessage(state, msg, pieceIndex); err != nil {
				log.Warn().Msgf("Error handling first message from %s: %v", peer.String(), err)
				continue
			}
		}

		// Check if peer has the piece we want
		if !state.bitfield.HasPiece(pieceIndex) {
			log.Debug().Msgf("Peer %s does not have piece %d", peer.String(), pieceIndex)
			continue // Try next peer
		}
		log.Debug().Msgf("Peer %s has piece %d", peer.String(), pieceIndex)

		// Send Interested message
		interestedMsg := torrent.Message{Type: torrent.MsgInterested}
		_, err = state.conn.Write(interestedMsg.Serialize())
		if err != nil {
			log.Warn().Msgf("Failed to send Interested to %s: %v", peer.String(), err)
			continue
		}
		log.Debug().Msgf("Sent Interested to %s", peer.String())

		// 6 & 7. Request blocks and receive piece data
		pieceData, err := downloadPieceFromChokedPeer(state, tor, pieceIndex, pieceLength)
		if err != nil {
			log.Warn().Msgf("Failed to download piece %d from %s: %v", pieceIndex, peer.String(), err)
			continue // Try next peer
		}

		// 8. Piece successfully downloaded
		log.Info().Msgf("Successfully downloaded piece %d from peer %s", pieceIndex, peer.String())
		return pieceData, nil
	}

	// 9. If piece could not be downloaded from any peer:
	return nil, fmt.Errorf("failed to download piece %d from any available peer", pieceIndex)
}

// readMessageWithTimeout reads a message with a specific timeout.
func readMessageWithTimeout(conn net.Conn, timeout time.Duration) (*torrent.Message, error) {
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{}) // Clear deadline
	return torrent.ReadMessage(conn)
}

// downloadPieceFromChokedPeer handles the message loop for downloading a piece
// after the initial handshake and bitfield exchange.
func downloadPieceFromChokedPeer(state *peerConnectionState, tor *torrent.Torrent, pieceIndex int, pieceLength int64) ([]byte, error) {
	pieceBuf := make([]byte, pieceLength)
	downloadedBytes := int64(0)
	requestedBlocks := 0
	receivedBlocks := 0
	backlog := 0 // Number of requests currently pending

	// Calculate total blocks needed
	totalBlocks := int(pieceLength+torrent.BlockSize-1) / torrent.BlockSize

	// Timeout for the entire piece download from this peer
	pieceDownloadTimeout := time.After(60 * time.Second)

	for receivedBlocks < totalBlocks {
		select {
		case <-pieceDownloadTimeout:
			return nil, fmt.Errorf("piece download timed out")
		default:
			// Only send requests if not choked and backlog is low
			if !state.peerChoked {
				for backlog < torrent.MaxBacklog && requestedBlocks < totalBlocks {
					blockOffset := int64(requestedBlocks) * torrent.BlockSize
					blockSize := torrent.BlockSize
					// Adjust size for the last block
					if blockOffset+int64(blockSize) > pieceLength {
						blockSize = int(pieceLength - blockOffset)
					}

					err := state.sendRequest(uint32(pieceIndex), uint32(blockOffset), uint32(blockSize))
					if err != nil {
						return nil, fmt.Errorf("failed to send request: %w", err)
					}
					requestedBlocks++
					backlog++
					log.Trace().Msgf("Requested block %d/%d (offset %d, size %d) for piece %d from %s",
						requestedBlocks, totalBlocks, blockOffset, blockSize, pieceIndex, state.peer.String())
				}
			}

			// Read the next message from the peer
			// Use a shorter timeout for individual messages once unchoked
			readTimeout := 30 * time.Second
			if state.peerChoked {
				readTimeout = 10 * time.Second // Longer timeout while waiting for unchoke
			}
			msg, err := readMessageWithTimeout(state.conn, readTimeout)
			if err != nil {
				return nil, fmt.Errorf("failed to read message: %w", err)
			}

			if err := handleMessage(state, msg, pieceIndex); err != nil {
				return nil, fmt.Errorf("error handling message: %w", err)
			}

			// Handle Piece message
			if msg.Type == torrent.MsgPiece {
				index, begin, data, err := torrent.ParsePiece(msg.Payload)
				if err != nil {
					return nil, fmt.Errorf("failed to parse piece message: %w", err)
				}
				if int(index) != pieceIndex {
					log.Warn().Msgf("Received piece message for wrong index %d (expected %d) from %s",
						index, pieceIndex, state.peer.String())
					continue // Ignore
				}
				if int64(begin)+int64(len(data)) > pieceLength {
					return nil, fmt.Errorf("received block data exceeds piece length (begin %d, len %d, pieceLen %d)",
						begin, len(data), pieceLength)
				}

				copy(pieceBuf[begin:], data)
				downloadedBytes += int64(len(data))
				receivedBlocks++
				backlog--
				log.Trace().Msgf("Received block (offset %d, size %d) for piece %d from %s. Total %d/%d blocks, %d/%d bytes",
					begin, len(data), pieceIndex, state.peer.String(), receivedBlocks, totalBlocks, downloadedBytes, pieceLength)
			}
		}
	}

	if downloadedBytes != pieceLength {
		return nil, fmt.Errorf("downloaded size mismatch: expected %d, got %d", pieceLength, downloadedBytes)
	}

	return pieceBuf, nil
}

// handleMessage processes incoming messages from a peer.
func handleMessage(state *peerConnectionState, msg *torrent.Message, currentPieceIndex int) error {
	switch msg.Type {
	case torrent.MsgKeepAlive:
		log.Trace().Msgf("Received KeepAlive from %s", state.peer.String())
	case torrent.MsgChoke:
		log.Debug().Msgf("Received Choke from %s", state.peer.String())
		state.peerChoked = true
	case torrent.MsgUnchoke:
		log.Debug().Msgf("Received Unchoke from %s", state.peer.String())
		state.peerChoked = false
	case torrent.MsgInterested:
		log.Trace().Msgf("Received Interested from %s (ignoring)", state.peer.String())
		// We are the downloader, typically don't need to handle peer's interest
	case torrent.MsgNotInterested:
		log.Trace().Msgf("Received NotInterested from %s (ignoring)", state.peer.String())
	case torrent.MsgHave:
		index, err := torrent.ParseHave(msg.Payload)
		if err != nil {
			return fmt.Errorf("failed to parse Have message from %s: %w", state.peer.String(), err)
		}
		if state.bitfield != nil {
			state.bitfield.SetPiece(int(index))
			log.Trace().Msgf("Received Have for piece %d from %s", index, state.peer.String())
		} else {
			log.Warn().Msgf("Received Have message before Bitfield from %s", state.peer.String())
			// Handle appropriately, maybe request bitfield again or disconnect
		}
	case torrent.MsgBitfield:
		// Should have been handled earlier, but log if received again
		log.Warn().Msgf("Received unexpected Bitfield message from %s", state.peer.String())
	case torrent.MsgRequest:
		log.Trace().Msgf("Received Request from %s (ignoring)", state.peer.String())
		// We are the downloader, typically don't fulfill requests
	case torrent.MsgPiece:
		// Handled in the downloadPieceFromChokedPeer loop
		// No action needed here for MsgPiece, just prevents falling into default

	case torrent.MsgCancel:
		log.Trace().Msgf("Received Cancel from %s (ignoring)", state.peer.String())
	case torrent.MsgPort:
		log.Trace().Msgf("Received Port from %s (ignoring)", state.peer.String())
	default:
		log.Warn().Msgf("Received unknown message type %d from %s", msg.Type, state.peer.String())
	}
	return nil
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
