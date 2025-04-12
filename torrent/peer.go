package torrent

import (
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
)

type Peer struct {
	ID   string
	IP   string
	Port uint16
}

func PeerMe() *Peer {
	// genarete a 20 byte random peer ID
	id := make([]byte, 20)
	rand.Read(id)

	return &Peer{
		ID:   string(id),
		IP:   externalIP(),
		Port: 6881,
	}
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

func externalIP() string {
	ipService := "https://api.ipify.org/"

	resp, err := http.Get(ipService)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return string(respBytes)
}
