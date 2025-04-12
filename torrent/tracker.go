package torrent

import (
	"fmt"
	"net/url"
)

type ITracker interface {
	GetPeers(tor *Torrent, me *Peer) ([]*Peer, error)
	Announce() string
	LastCheck() int64
	NextCheck() int64
	LastError() error
	Seeders() int
	Leechers() int
}

func NewTracker(announce string) (ITracker, error) {
	url, err := url.Parse(announce)
	if err != nil {
		return nil, err
	}
	protocol := url.Scheme
	if protocol == "" {
		protocol = "http"
	}
	switch protocol {
	case "https":
		fallthrough
	case "http":
		return NewHTTPTracker(announce), nil
	case "udp":
		return NewUDPTracker(announce), nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocol)
	}
}
