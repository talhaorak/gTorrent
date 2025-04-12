package torrent

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// Constants for BitTorrent protocol
const (
	ProtocolIdentifier = "BitTorrent protocol"
	BlockSize          = 16 * 1024 // 16 KiB block size for requests
	MaxBacklog         = 5         // Number of block requests to keep pipelined
)

// MessageType identifies the type of a BitTorrent message.
type MessageType uint8

// Message types defined by the BitTorrent protocol.
const (
	MsgChoke         MessageType = 0
	MsgUnchoke       MessageType = 1
	MsgInterested    MessageType = 2
	MsgNotInterested MessageType = 3
	MsgHave          MessageType = 4
	MsgBitfield      MessageType = 5
	MsgRequest       MessageType = 6
	MsgPiece         MessageType = 7
	MsgCancel        MessageType = 8
	MsgPort          MessageType = 9   // Typically not used by download clients
	MsgKeepAlive     MessageType = 255 // Special case, no ID, zero length
)

// Message represents a generic BitTorrent message.
type Message struct {
	Type    MessageType
	Payload []byte
}

// Handshake represents the initial handshake message.
type Handshake struct {
	Pstrlen  uint8
	Pstr     string
	Reserved [8]byte
	InfoHash [20]byte
	PeerID   [20]byte
}

// NewHandshake creates a new Handshake message.
func NewHandshake(infoHash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstrlen:  uint8(len(ProtocolIdentifier)),
		Pstr:     ProtocolIdentifier,
		InfoHash: infoHash,
		PeerID:   peerID,
	}
}

// Serialize converts the Handshake struct into a byte slice.
func (h *Handshake) Serialize() []byte {
	buf := make([]byte, 49+len(h.Pstr))
	buf[0] = h.Pstrlen
	copy(buf[1:], h.Pstr)
	copy(buf[1+len(h.Pstr)+8:], h.InfoHash[:])
	copy(buf[1+len(h.Pstr)+8+20:], h.PeerID[:])
	return buf
}

// ReadHandshake reads and parses a Handshake message from the reader.
func ReadHandshake(r io.Reader) (*Handshake, error) {
	lengthBuf := make([]byte, 1)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}
	pstrlen := int(lengthBuf[0])
	if pstrlen == 0 {
		return nil, fmt.Errorf("pstrlen cannot be 0")
	}

	handshakeBuf := make([]byte, 48+pstrlen)
	_, err = io.ReadFull(r, handshakeBuf)
	if err != nil {
		return nil, err
	}

	var infoHash, peerID [20]byte
	pstr := string(handshakeBuf[:pstrlen])
	// reserved := handshakeBuf[pstrlen : pstrlen+8] // We don't use reserved bytes yet
	copy(infoHash[:], handshakeBuf[pstrlen+8:pstrlen+8+20])
	copy(peerID[:], handshakeBuf[pstrlen+8+20:])

	h := &Handshake{
		Pstrlen:  uint8(pstrlen),
		Pstr:     pstr,
		InfoHash: infoHash,
		PeerID:   peerID,
	}
	// Copy reserved bytes
	copy(h.Reserved[:], handshakeBuf[pstrlen:pstrlen+8])

	return h, nil
}

// PerformHandshake performs the BitTorrent handshake with a peer.
func PerformHandshake(conn net.Conn, tor *Torrent, selfPeerID [20]byte) (*Handshake, error) {
	conn.SetDeadline(time.Now().Add(5 * time.Second)) // Set timeout for handshake
	defer conn.SetDeadline(time.Time{})               // Clear timeout

	req := NewHandshake(tor.InfoHash, selfPeerID)
	_, err := conn.Write(req.Serialize())
	if err != nil {
		return nil, fmt.Errorf("failed to send handshake: %w", err)
	}

	res, err := ReadHandshake(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read handshake response: %w", err)
	}

	// Validate handshake response
	if res.Pstr != ProtocolIdentifier {
		return nil, fmt.Errorf("invalid protocol identifier from peer: %s", res.Pstr)
	}
	if res.InfoHash != tor.InfoHash {
		return nil, fmt.Errorf("infohash mismatch")
	}

	// Note: We don't strictly need to check the peer ID in the response,
	// but it's available in res.PeerID if needed.

	return res, nil
}

// Serialize converts a Message struct into a byte slice for sending.
// Format: <length prefix (4 bytes)><message ID (1 byte)><payload>
// KeepAlive messages have length 0 and no ID or payload.
func (m *Message) Serialize() []byte {
	if m.Type == MsgKeepAlive {
		return make([]byte, 4) // Length prefix of 0
	}
	length := uint32(1 + len(m.Payload)) // Message ID + Payload length
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = byte(m.Type)
	copy(buf[5:], m.Payload)
	return buf
}

// ReadMessage reads a message from the connection.
func ReadMessage(r io.Reader) (*Message, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	// KeepAlive message
	if length == 0 {
		return &Message{Type: MsgKeepAlive}, nil
	}

	messageBuf := make([]byte, length)
	_, err = io.ReadFull(r, messageBuf)
	if err != nil {
		return nil, err
	}

	m := &Message{
		Type:    MessageType(messageBuf[0]),
		Payload: messageBuf[1:],
	}
	return m, nil
}

// FormatRequest creates the payload for a Request message.
func FormatRequest(index, begin, length uint32) []byte {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], index)
	binary.BigEndian.PutUint32(payload[4:8], begin)
	binary.BigEndian.PutUint32(payload[8:12], length)
	return payload
}

// ParsePiece extracts index, begin, and data from a Piece message payload.
func ParsePiece(payload []byte) (index, begin uint32, data []byte, err error) {
	if len(payload) < 8 {
		err = fmt.Errorf("piece payload too short: %d bytes", len(payload))
		return
	}
	index = binary.BigEndian.Uint32(payload[0:4])
	begin = binary.BigEndian.Uint32(payload[4:8])
	data = payload[8:]
	return
}

// ParseHave extracts the piece index from a Have message payload.
func ParseHave(payload []byte) (index uint32, err error) {
	if len(payload) != 4 {
		err = fmt.Errorf("have payload invalid length: %d", len(payload))
		return
	}
	index = binary.BigEndian.Uint32(payload)
	return
}

// Bitfield represents the pieces a peer has.
type Bitfield []byte

// HasPiece checks if the bitfield indicates the peer has a specific piece.
func (bf Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return false
	}
	return bf[byteIndex]>>(7-offset)&1 != 0
}

// SetPiece marks a piece as available in the bitfield.
func (bf Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return // Index out of bounds
	}
	bf[byteIndex] |= 1 << (7 - offset)
}
