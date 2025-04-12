package torrent

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"gtorrent/bencode"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestAllTorrentFiles(t *testing.T) {
	torrentFiles := make([]string, 0)
	// read all *.torrent files in the current directory
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if strings.HasSuffix(file.Name(), ".torrent") {
			torrentFiles = append(torrentFiles, file.Name())
		}
	}

	if len(torrentFiles) == 0 {
		t.Fatal("No torrent files found.")
		return

	}

	for _, torrentFile := range torrentFiles {
		t.Run(torrentFile, func(t *testing.T) {
			testBody(t, torrentFile)
		})
	}
}

func TestParseTorrent(t *testing.T) {
	filename := "../torrent/meditations_marcus_aurelius.torrent"
	testBody(t, filename)
}

func testBody(t *testing.T, filename string) {
	file, err := os.Open(filename)
	if err != nil {
		t.Error(err)
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		t.Error(err)
		return
	}
	data, _, err := bencode.Decode(content)
	if err != nil {
		t.Error(err)
		return
	}
	torrent := TorrentFromBencodeData(data)

	filenameJson := filename + ".json"
	fileJson, err := os.Open(filenameJson)
	if err != nil {
		t.Error(err)
		return
	}
	contentJson, err := io.ReadAll(fileJson)
	if err != nil {
		t.Error(err)
		return
	}
	var torrentJson map[string]interface{}
	err = json.Unmarshal(contentJson, &torrentJson)
	if err != nil {
		t.Error(err)
		return
	}

	torrentInfoJson := torrentJson["info"].(map[string]interface{})

	// announce and announce-list
	announceList := make([]string, 0)
	if jsonAnnounceList, ok := torrentJson["announce-list"].([]interface{}); ok {
		for _, announceListData := range jsonAnnounceList {
			announceListDataList := announceListData.([]interface{})
			for _, announceData := range announceListDataList {
				announceList = append(announceList, announceData.(string))
			}
		}
	} else if jsonAnnounceListStr, ok := torrentJson["announce-list"].(string); ok {
		announceList = append(announceList, jsonAnnounceListStr)
	}

	if jsonAnnounce, ok := torrentJson["announce"].(string); ok {
		if !slices.Contains(announceList, jsonAnnounce) {
			announceList = append(announceList, jsonAnnounce)
		}
	}

	if len(torrent.AnnounceList) != len(announceList) {
		t.Errorf("Expected AnnounceList to have %d elements, got %d", len(announceList), len(torrent.AnnounceList))
	}

	for i, announce := range torrent.AnnounceList {
		if announce != announceList[i] {
			t.Errorf("Expected AnnounceList[%d] to be %s, got %s", i, announceList[i], announce)
		}
	}

	// name
	if torrent.Name != torrentInfoJson["name"].(string) {
		t.Errorf("Expected Name to be %s, got %s", torrentInfoJson["name"].(string), torrent.Name)
	}

	// url-list
	urlList := make([]string, 0)
	if jsonUrlList, ok := torrentJson["url-list"].([]interface{}); ok {
		for _, url := range jsonUrlList {
			urlList = append(urlList, url.(string))
		}
	} else if jsonUrlListStr, ok := torrentInfoJson["url-list"].(string); ok {
		urlList = append(urlList, jsonUrlListStr)
	}

	if len(torrent.UrlList) != len(urlList) {
		t.Errorf("Expected UrlList to have %d elements, got %d", len(urlList), len(torrent.UrlList))
	}

	for i, url := range torrent.UrlList {
		if url != urlList[i] {
			t.Errorf("Expected UrlList[%d] to be %s, got %s", i, urlList[i], url)
		}
	}

	// created-by
	if jsonCreatedBy, ok := torrentJson["created-by"].(string); ok {
		if torrent.CreatedBy != jsonCreatedBy {
			t.Errorf("Expected CreatedBy to be %s, got %s", jsonCreatedBy, torrent.CreatedBy)
		}
	}

	// comment
	if jsonComment, ok := torrentJson["comment"].(string); ok {
		if torrent.Comment != jsonComment {
			t.Errorf("Expected Comment to be %s, got %s", jsonComment, torrent.Comment)
		}
	}

	// created-at
	if jsonCreatedAt, ok := torrentJson["created-at"].(float64); ok {
		if torrent.CreatedAt != int64(jsonCreatedAt) {
			t.Errorf("Expected CreatedAt to be %d, got %d", int64(jsonCreatedAt), torrent.CreatedAt)
		}
	}

	// file-list
	if jsonFiles, ok := torrentInfoJson["files"].([]interface{}); ok {
		if len(torrent.FileList) != len(jsonFiles) {
			t.Errorf("Expected Files to have %d elements, got %d", len(jsonFiles), len(torrent.FileList))
		}
		for i, file := range torrent.FileList {
			jsonFile := jsonFiles[i].(map[string]interface{})
			if file.Length != int64(jsonFile["length"].(float64)) {
				t.Errorf("Expected Files[%d].Length to be %d, got %d", i, int64(jsonFile["length"].(float64)), file.Length)
			}
			var path strings.Builder
			for i, pathPart := range jsonFile["path"].([]interface{}) {
				path.WriteString(pathPart.(string))
				if i < len(jsonFile["path"].([]interface{}))-1 {
					path.WriteString("/")
				}
			}
			if file.Path != path.String() {
				t.Errorf("Expected Files[%d].Path to be %s, got %s", i, path.String(), file.Path)
			}
		}
	} else {
		// single file mode
		if len(torrent.FileList) != 1 {
			t.Errorf("Expected Files to have 1 element, got %d", len(torrent.FileList))
		}
		if torrent.FileList[0].Length != int64(torrentInfoJson["length"].(float64)) {
			t.Errorf("Expected Files[0].Length to be %d, got %d", int64(torrentInfoJson["length"].(float64)), torrent.FileList[0].Length)
		}
		if torrent.FileList[0].Path != torrent.Name {
			t.Errorf("Expected Files[0].Path to be %s, got %s", torrent.Name, torrent.FileList[0].Path)
		}
	}

	// is private
	if jsonPrivate, ok := torrentInfoJson["private"].(float64); ok {
		if torrent.IsPrivate != (jsonPrivate == 1) {
			t.Errorf("Expected IsPrivate to be %t, got %t", jsonPrivate == 1, torrent.IsPrivate)
		}
	}

	// test info hash
	dataDict := data.AsDict()
	infoData := dataDict["info"]
	hash := fmt.Sprintf("%x", sha1.Sum(infoData.ToBytes()))
	if torrent.InfoHashString() != string(hash) {
		t.Errorf("Expected InfoHash to be %x, got %x", hash, torrent.InfoHash)
	}

	// test torrent file encode
	bData, _, err := bencode.Decode(content)
	if err != nil {
		t.Error(err)
	}

	bdData := bData.ToBytes()

	// compare the original content with the re-encoded content
	if !slices.Equal(content, bdData) {
		t.Errorf("Expected re-encoded content to be equal to the original content")
	}

}

func TestVerifyTorrent(t *testing.T) {
	filename := "../example/meditations_marcus_aurelius.torrent"

	err := VerifyTorrent(filename, "../example")
	if err != nil {
		t.Error(err)
	}
}

func TestVerifyTorrent2(t *testing.T) {
	filename := "../torrent/debian-12.5.0-arm64-DVD-1.iso.torrent"

	err := VerifyTorrent(filename, "../torrent")
	if err != nil {
		t.Error(err)
	}
}

func TestVerifyTorrent3(t *testing.T) {
	filename := "../torrent/Interstellar.torrent"

	err := VerifyTorrent(filename, "/Users/talhaorak/Movies/Interstellar (2014) [2160p] [4K] [BluRay] [5.1] [YTS.MX]")
	if err != nil {
		t.Error(err)
	}
}
