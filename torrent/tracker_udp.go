package torrent

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"time"

	"net"
	"net/url"
)

type udpTracker struct {
	announceURL  string
	lastCheck    int64
	nextCheck    int64
	lastError    error
	conn         *net.UDPConn
	connectionID int64
	leechers     int32
	seeders      int32
	peers        []*Peer
}

// actions enum:
const (
	actionConnect  = 0
	actionAnnounce = 1
	actionScrape   = 2
	actionError    = 3
)

// // errors enum:
// const (
// 	errorGeneric = 100
// 	errorParse   = 101
// 	errorUnknown = 102
// )

// event enum:
const (
	eventNone      = 0
	eventCompleted = 1
	eventStarted   = 2
	eventStopped   = 3
)

const connectionID = 0x41727101980

func NewUDPTracker(announce string) ITracker {
	return &udpTracker{
		announceURL: announce,
		peers:       make([]*Peer, 0),
	}
}

func (t *udpTracker) GetPeers(tor *Torrent, me *Peer) ([]*Peer, error) {

	err := t.connect()
	if err != nil {
		t.lastError = err
		return t.peers, err
	}
	defer t.disconnect()
	err = t.acquireConnectionID()
	if err != nil {
		t.lastError = err
		return t.peers, err
	}

	err = t.scrape(tor)
	if err != nil {
		t.lastError = err
		return t.peers, err
	}

	err = t.announce(tor, me)
	if err != nil {
		t.lastError = err
		return t.peers, err
	}

	return t.peers, nil
}

func (t *udpTracker) connect() error {
	url, err := url.Parse(t.announceURL)
	if err != nil {
		return err
	}
	addr, err := net.ResolveUDPAddr("udp", url.Host)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	t.conn = conn
	t.conn.SetDeadline(time.Now().Add(15 * time.Second))
	return nil
}

func (t *udpTracker) disconnect() {
	t.conn.Close()

}

func (t *udpTracker) acquireConnectionID() error {
	transactionID := rand.Int31()
	// obtain a connection id from the tracker
	request := struct {
		ConnectionID int64
		Action       int32
		Transaction  int32
	}{
		ConnectionID: connectionID,
		Action:       actionConnect,
		Transaction:  transactionID,
	}

	// serialize the request into a buffer
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, request)
	if err != nil {
		return err
	}

	// send the request to the tracker
	_, err = t.conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	// read the response
	response := struct {
		Action       int32
		Transaction  int32
		ConnectionID int64
	}{}
	err = binary.Read(t.conn, binary.BigEndian, &response)
	if err != nil {
		return err
	}
	if response.Transaction != transactionID {
		return fmt.Errorf("transaction ID mismatch")
	}
	if response.Action != 0 {
		return fmt.Errorf("unexpected action: %d", response.Action)
	}
	t.connectionID = response.ConnectionID
	return nil
}

func (t *udpTracker) announce(tor *Torrent, me *Peer) error {

	transactionID := rand.Int31()
	// announce to the tracker

	userIDArray := [20]byte{}
	copy(userIDArray[:], me.ID)

	request := struct {
		ConnectionID int64
		Action       int32
		Transaction  int32
		InfoHash     [20]byte
		PeerID       [20]byte
		Downloaded   int64
		Left         int64
		Uploaded     int64
		Event        int32
		IP           int32
		Key          int32
		NumWant      int32
		Port         uint16
	}{
		ConnectionID: t.connectionID,
		Action:       actionAnnounce,
		Transaction:  transactionID,
		InfoHash:     tor.InfoHash,
		PeerID:       userIDArray,
		Downloaded:   0,
		Left:         tor.Length,
		Uploaded:     0,
		Event:        eventStarted,
		IP:           0,
		Key:          0,
		NumWant:      -1,
		Port:         uint16(me.Port),
	}

	// serialize the request into a buffer
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, request)
	if err != nil {
		return err
	}

	// send the request to the tracker
	_, err = t.conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	readBytes := make([]byte, 1024)
	n, err := t.conn.Read(readBytes)
	if err != nil {
		return err
	}
	readBytes = readBytes[:n]

	// read the response
	response := struct {
		Action      int32
		Transaction int32
		Interval    int32
		Leechers    int32
		Seeders     int32
	}{}

	err = binary.Read(bytes.NewReader(readBytes), binary.BigEndian, &response)
	if err != nil {
		return err
	}

	if response.Transaction != transactionID {
		return fmt.Errorf("transaction ID mismatch")
	}
	if response.Action != actionAnnounce {
		return fmt.Errorf("unexpected action: %d", response.Action)
	}
	t.leechers = response.Leechers
	t.seeders = response.Seeders

	t.peers = make([]*Peer, 0)

	readBytes = readBytes[20:]
	for len(readBytes) > 0 {
		ip := net.IPv4(readBytes[0], readBytes[1], readBytes[2], readBytes[3])
		port := uint16(readBytes[4])<<8 + uint16(readBytes[5])
		peer := Peer{
			IP:   ip.String(),
			Port: port,
		}

		t.peers = append(t.peers, &peer)
		readBytes = readBytes[6:]
	}
	t.lastCheck = time.Now().Unix()
	t.nextCheck = t.lastCheck + int64(response.Interval)
	return nil
}

func (t *udpTracker) scrape(tor *Torrent) error {
	transactionID := rand.Int31()
	// announce to the tracker

	request := struct {
		ConnectionID int64
		Action       int32
		Transaction  int32
		InfoHash     [20]byte
	}{
		ConnectionID: t.connectionID,
		Action:       actionScrape,
		Transaction:  transactionID,
		InfoHash:     tor.InfoHash,
	}

	// serialize the request into a buffer
	var buf bytes.Buffer
	err := binary.Write(&buf, binary.BigEndian, request)
	if err != nil {
		return err
	}

	// send the request to the tracker
	_, err = t.conn.Write(buf.Bytes())
	if err != nil {
		return err
	}

	readBytes := make([]byte, 1024)
	n, err := t.conn.Read(readBytes)
	if err != nil {
		return err
	}
	readBytes = readBytes[:n]

	// read the response
	response := struct {
		Action      int32
		Transaction int32
		Seeders     int32
		Completed   int32
		Leechers    int32
	}{}

	err = binary.Read(bytes.NewReader(readBytes), binary.BigEndian, &response)
	if err != nil {
		return err
	}

	if response.Transaction != transactionID {
		return fmt.Errorf("transaction ID mismatch")
	}

	if response.Action != actionScrape {
		return fmt.Errorf("unexpected action: %d", response.Action)
	}

	t.seeders = response.Seeders

	t.leechers = response.Leechers
	t.lastCheck = time.Now().Unix()
	return nil
}

func (t *udpTracker) Announce() string {
	return t.announceURL
}

func (t *udpTracker) LastCheck() int64 {
	return t.lastCheck
}

func (t *udpTracker) NextCheck() int64 {
	return t.nextCheck
}

func (t *udpTracker) LastError() error {
	return t.lastError
}

func (t *udpTracker) Seeders() int {
	return int(t.seeders)
}

func (t *udpTracker) Leechers() int {
	return int(t.leechers)
}
