package torrent

import (
	"fmt"
	"gtorrent/bencode"

	"time"

	"github.com/go-resty/resty/v2"
)

type httpTracker struct {
	announceURL string
	lastCheck   int64
	nextCheck   int64
	lastError   error
	lastWarning string
	seeders     int
	leechers    int
}

func NewHTTPTracker(announce string) ITracker {
	return &httpTracker{
		announceURL: announce,
	}
}

func (t *httpTracker) Announce() string {
	return t.announceURL
}

func (t *httpTracker) LastCheck() int64 {
	return t.lastCheck
}

func (t *httpTracker) NextCheck() int64 {
	return t.nextCheck
}

func (t *httpTracker) LastError() error {
	return t.lastError
}

func (t *httpTracker) Seeders() int {
	return t.seeders
}

func (t *httpTracker) Leechers() int {
	return t.leechers
}

func (t *httpTracker) GetPeers(tor *Torrent, me *Peer) ([]*Peer, error) {
	peers := make([]*Peer, 0)
	cli := resty.New()

	resp, err := cli.R().
		SetQueryParam("info_hash", string(tor.InfoHash[:])).
		SetQueryParam("peer_id", me.ID).
		SetQueryParam("ip", me.IP).
		SetQueryParam("port", fmt.Sprintf("%d", me.Port)).
		SetQueryParam("uploaded", "0").
		SetQueryParam("downloaded", "0").
		SetQueryParam("left", fmt.Sprintf("%d", tor.Length)).
		SetQueryParam("event", "started").
		Get(t.announceURL)
	if err != nil {
		err = fmt.Errorf("status code: %d, error: %s", resp.StatusCode(), err.Error())
		t.lastError = err
		return peers, err
	}
	t.lastCheck = time.Now().Unix()
	if resp.StatusCode() != 200 {
		err = fmt.Errorf("status code: %d, error: %s", resp.StatusCode(), resp.String())
		t.lastError = err
		return peers, err
	}
	// Parse the response
	response, _, err := bencode.Decode(resp.Body())
	if err != nil {
		err = fmt.Errorf("status code: %d, error: %s", resp.StatusCode(), err.Error())
		t.lastError = err
		return peers, err
	}
	// respStr := response.ToJSON()
	// log.Debug().Msgf("Response: %s", respStr)
	respDict := response.AsDict()

	if failureReason, ok := respDict["failure reason"]; ok {
		err = fmt.Errorf(failureReason.AsString())
		t.lastError = err
		return peers, err
	}

	if complete, ok := respDict["complete"]; ok {
		t.seeders = int(complete.AsInt())
	}

	// if downloaded, ok := respDict["downloaded"]; ok {
	// 	t.seeders = int(downloaded.AsInt())
	// }

	if leechers, ok := respDict["incomplete"]; ok {
		t.leechers = int(leechers.AsInt())
	}

	if interval, ok := respDict["interval"]; ok {
		t.nextCheck = time.Now().Unix() + int64(interval.AsInt())
	}

	if peersList, ok := respDict["peers"]; ok {
		if peersList.Type == bencode.STRING {
			peersList := peersList.AsString()
			for i := 0; i < len(peersList); i += 6 {
				peer := &Peer{
					IP:   fmt.Sprintf("%d.%d.%d.%d", peersList[i], peersList[i+1], peersList[i+2], peersList[i+3]),
					Port: uint16(int(peersList[i+4])<<8 + int(peersList[i+5])),
				}
				peers = append(peers, peer)
			}
		} else if peersList.Type == bencode.LIST {
			for _, peerData := range peersList.AsList() {
				peerDict := peerData.AsDict()
				peer := &Peer{
					IP:   peerDict["ip"].AsString(),
					Port: uint16(peerDict["port"].AsInt()),
				}
				peers = append(peers, peer)
			}
		}

	}

	if lastWarning, ok := respDict["warning message"]; ok {
		t.lastWarning = lastWarning.AsString()
	}
	return peers, nil
}
