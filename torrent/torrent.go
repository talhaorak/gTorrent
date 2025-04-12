package torrent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"gtorrent/bencode"
	"gtorrent/utils"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Torrent struct {
	AnnounceList []string
	Name         string
	UrlList      []string
	CreatedBy    string
	Comment      string
	CreatedAt    int64
	FileList     []*File
	PieceLength  int64
	Pieces       []string
	InfoHash     [20]byte
	Length       int64
	IsPrivate    bool
}

func NewTorrent() *Torrent {
	return &Torrent{
		AnnounceList: make([]string, 0),
		UrlList:      make([]string, 0),
		FileList:     make([]*File, 0),
		Pieces:       make([]string, 0),
	}
}

func (t *Torrent) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Name: %s\n", t.Name))
	sb.WriteString(fmt.Sprintf("  InfoHash: %s\n", t.InfoHash))
	sb.WriteString(fmt.Sprintf("  Length: %s\n", utils.FormatBytes(t.Length)))

	sb.WriteString("  AnnounceList:\n")
	for _, announce := range t.AnnounceList {
		sb.WriteString(fmt.Sprintf("     %s\n", announce))
	}

	sb.WriteString("  UrlList:\n")
	for _, url := range t.UrlList {
		sb.WriteString(fmt.Sprintf("     %s\n", url))
	}
	sb.WriteString(fmt.Sprintf("  CreatedBy: %s\n", t.CreatedBy))
	sb.WriteString(fmt.Sprintf("  Comment: %s\n", t.Comment))
	sb.WriteString(fmt.Sprintf("  CreatedAt: %s\n", time.Unix(t.CreatedAt, 0).String()))
	sb.WriteString("  FileList:\n")
	for _, file := range t.FileList {
		sb.WriteString(fmt.Sprintf("     %s\n", file.String()))
	}
	sb.WriteString(fmt.Sprintf("  PieceLength: %s\n", utils.FormatBytes(t.PieceLength)))
	// sb.WriteString(fmt.Sprintf("  Pieces: %v\n", t.Pieces))

	return sb.String()
}

func (t *Torrent) InfoHashString() string {
	return hex.EncodeToString(t.InfoHash[:])
}

type File struct {
	Length          int64
	Path            string
	FirstPieceIndex int
	LastPieceIndex  int
}

func NewFile(length int64, path string) *File {
	return &File{
		Length: length,
		Path:   path,
	}
}

func (f *File) String() string {
	return fmt.Sprintf("Path: %s(%s)", f.Path, utils.FormatBytes(f.Length))
}

// TorrentFromBencodeData converts bencode data into a Torrent struct.
// It extracts all torrent metadata including announce lists, file information,
// piece hashes, and other properties from the bencode data.
// Returns nil if the input data is nil.
func TorrentFromBencodeData(data *bencode.Data) *Torrent {
	if data == nil {
		return nil
	}
	torrent := NewTorrent()
	rootDict := data.AsDict()
	infoDict := rootDict["info"].AsDict()

	// announce-list
	if announceList, ok := rootDict["announce-list"]; ok {
		announceListData := announceList.AsList()
		for _, announceData := range announceListData {
			announceList := announceData.AsList()
			for _, announce := range announceList {
				torrent.AnnounceList = append(torrent.AnnounceList, announce.AsString())
			}
		}
	}

	// announce
	if announce, ok := rootDict["announce"]; ok {
		if !slices.Contains(torrent.AnnounceList, announce.AsString()) {
			torrent.AnnounceList = append(torrent.AnnounceList, announce.AsString())
		}
	}

	// name
	if name, ok := infoDict["name"]; ok {
		torrent.Name = name.AsString()
	}

	// url-list
	if urlList, ok := rootDict["url-list"]; ok {
		urlListData := urlList.AsList()
		for _, url := range urlListData {
			torrent.UrlList = append(torrent.UrlList, url.AsString())
		}
	}

	// comment
	if comment, ok := rootDict["comment"]; ok {
		torrent.Comment = comment.AsString()
	}

	// created by
	if createdBy, ok := rootDict["created by"]; ok {
		torrent.CreatedBy = createdBy.AsString()
	}

	// creation date
	if createdAt, ok := rootDict["creation date"]; ok {
		torrent.CreatedAt = createdAt.AsInt()
	}

	// files list
	if files, ok := infoDict["files"]; ok {
		filesData := files.AsList()
		for _, fileData := range filesData {
			fileDict := fileData.AsDict()
			file := NewFile(fileDict["length"].AsInt(), "")

			if filePath, ok := fileDict["path"]; ok {
				pathData := filePath.AsList()
				for i, path := range pathData {
					// join path with "/"
					file.Path += path.AsString()
					if i < len(pathData)-1 {
						file.Path += "/"
					}
				}
			}

			torrent.FileList = append(torrent.FileList, file)
			torrent.Length += file.Length
		}
	} else {
		// single file mode
		torrent.Length = infoDict["length"].AsInt()
		file := NewFile(torrent.Length, torrent.Name)
		torrent.FileList = append(torrent.FileList, file)
	}

	// piece length
	if pieceLength, ok := infoDict["piece length"]; ok {
		torrent.PieceLength = pieceLength.AsInt()
	} else {
		torrent.PieceLength = 0

	}

	// pieces
	if pieces, ok := infoDict["pieces"]; ok {
		piecesData := pieces.AsBytes()
		for i := 0; i < len(piecesData); i += 20 {
			piece := fmt.Sprintf("%x", piecesData[i:i+20])
			torrent.Pieces = append(torrent.Pieces, piece)
		}
	}

	// is private
	if isPrivate, ok := infoDict["private"]; ok {
		torrent.IsPrivate = isPrivate.AsInt() == 1
	}

	// info hash
	infoData := rootDict["info"]
	hash := sha1.Sum(infoData.ToBytes())
	torrent.InfoHash = hash

	// put piece indices in the files
	pieceIndex := 0
	for _, file := range torrent.FileList {
		// calculate the number of pieces for this file
		pieceCount := file.Length / torrent.PieceLength
		if file.Length%torrent.PieceLength != 0 {
			pieceCount++
		}
		file.FirstPieceIndex = pieceIndex
		file.LastPieceIndex = pieceIndex + int(pieceCount) - 1
		pieceIndex += int(pieceCount)
	}

	return torrent
}

// TorrentFromBytes parses a byte slice containing torrent file data and converts it to a Torrent struct.
// This is typically used when reading a .torrent file from disk.
// It first decodes the bencode data and then converts it to a Torrent struct.
// Returns an error if the bencode data cannot be decoded.
func TorrentFromBytes(data []byte) (*Torrent, error) {
	bencodeData, _, err := bencode.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("error decoding torrent file: %s", err.Error())
	}
	return TorrentFromBencodeData(bencodeData), nil
}

// VerifyTorrent checks if the files described in a torrent file exist at the given contentPath
// and validates their integrity by comparing the SHA-1 hashes of each piece with those defined in the torrent.
// This function reads files piece by piece and computes hashes to verify integrity.
// Parameters:
//   - filename: Path to the .torrent file to verify
//   - contentPath: Path to the directory containing the downloaded files
//
// Returns an error if verification fails, or nil if all pieces match their expected hashes.
func VerifyTorrent(filename string, contentPath string) error {
	println("Opening torrent file: " + filename)

	// Open the torrent file
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	// Convert the bencoded data to a Torrent struct
	torrent, err := TorrentFromBytes(content)
	if err != nil {
		return err
	}

	// Verify the existence of the physical files
	for _, file := range torrent.FileList {
		filePath := filepath.Join(contentPath, file.Path)
		if _, err := os.Stat(filePath); err != nil {
			return err
		}
	}

	// Verify the integrity of the files
	/* Note: For the purposes of piece boundaries in the multi-file case,
	consider the file data as one long continuous stream, composed of the concatenation of
	each file in the order listed in the files list. The number of pieces and their boundaries
	are then determined in the same manner as the case of a single file.
	Pieces may overlap file boundaries.
	So we have this strategy:
	1. Open each file and read chunks in the size of the piece length
	2. if the last chunk is smaller than the piece length, append it to the next chunk
	3. Calculate the SHA1 hash of the chunk
	*/

	pieceLength := torrent.PieceLength
	pieceHashes := torrent.Pieces
	pieceIndex := 0
	piece := make([]byte, pieceLength)
	// Create a single reusable buffer for reading pieces
	pieceBuf := make([]byte, pieceLength)

	for fileIndex, file := range torrent.FileList {
		println("Checking " + file.Path)
		filePath := filepath.Join(contentPath, file.Path)
		f, err := os.Open(filePath)
		if err != nil {
			return err
		}

		// Process the file
		fileProcessingErr := func() error {
			defer f.Close() // Close inside the function scope when done with this file

			for {
				// Use our reusable buffer instead of creating a new one each time
				n, err := f.Read(pieceBuf)
				if err != nil {
					if err.Error() == "EOF" {
						break
					}
					return err
				}
				if n == 0 {
					break
				}
				if n < int(pieceLength) {
					if len(piece) < int(pieceLength) {
						piece = append(piece, pieceBuf[:n]...)
					} else {
						// Copy the data instead of reassigning
						copy(piece, pieceBuf[:n])
						// Ensure piece has the right length
						piece = piece[:n]
					}

					if fileIndex != len(torrent.FileList)-1 {
						break
					}
				} else {
					// Use our buffer directly
					piece = pieceBuf[:n]
				}

				hash := sha1.Sum(piece)
				hashStr := fmt.Sprintf("%x", hash)
				if hashStr != pieceHashes[pieceIndex] {
					return fmt.Errorf("piece %d is corrupted", pieceIndex)
				}
				pieceIndex++
				if pieceIndex == len(pieceHashes) {
					break
				}
			}
			return nil // Add explicit return nil
		}()

		// If there was an error processing this file, return it
		if fileProcessingErr != nil {
			return fileProcessingErr
		}
	}
	return nil
}
