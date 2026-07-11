package rcon

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// Source RCON protocol constants. See:
// https://developer.valvesoftware.com/wiki/Source_RCON_Protocol
const (
	execCommandType   int32 = 2 // SERVERDATA_EXECCOMMAND
	authType          int32 = 3 // SERVERDATA_AUTH
	responseValueType int32 = 0 // SERVERDATA_RESPONSE_VALUE
	authResponseType  int32 = 2 // SERVERDATA_AUTH_RESPONSE

	// commandID marks packets belonging to the real command, sentinelID marks
	// the trailing empty command used to detect the end of a multi-packet
	// response. They must differ so we can tell the two apart in the stream.
	commandID  int32 = 1
	sentinelID int32 = 2

	// maxPacketSize guards against a malformed/hostile size header causing a huge
	// allocation. Source RCON packets are at most ~4KB, so 64KB is generous.
	maxPacketSize = 64 * 1024

	// packetOverhead is the fixed part of a packet body-length field: the ID and
	// Type int32s (8 bytes) plus the two trailing null bytes.
	packetOverhead = 10
)

var errAuthFailed = errors.New("rcon: authentication failed")

// executeMultiPacket opens a dedicated TCP connection to a Source RCON server,
// authenticates, runs a single command, and returns its full output — correctly
// reassembling responses that the server splits across multiple packets.
//
// CS2 commands such as "maps *" produce output far larger than the ~4KB single
// packet limit, so the server streams several SERVERDATA_RESPONSE_VALUE packets.
// The pooled gorcon client only reads the first of these, which both truncates
// the result and desyncs the connection for later commands. To avoid both
// problems we use a throwaway connection here and the standard sentinel trick:
// after the real command we send an empty command with a distinct packet ID; the
// server answers commands in order, so the first packet echoing that sentinel ID
// marks the end of the real command's output.
func executeMultiPacket(host, password, command string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return "", fmt.Errorf("rcon dial %s: %w", host, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	r := bufio.NewReader(conn)

	if err := authenticate(conn, r, password); err != nil {
		return "", err
	}

	// Send the real command followed immediately by an empty sentinel command.
	if err := writePacket(conn, commandID, execCommandType, command); err != nil {
		return "", fmt.Errorf("rcon write command: %w", err)
	}
	if err := writePacket(conn, sentinelID, execCommandType, ""); err != nil {
		return "", fmt.Errorf("rcon write sentinel: %w", err)
	}

	var out bytes.Buffer
	for {
		id, typ, body, err := readPacket(r)
		if err != nil {
			// If we hit EOF after collecting output, treat what we have as the
			// full response — some servers close instead of echoing the sentinel.
			if errors.Is(err, io.EOF) && out.Len() > 0 {
				return out.String(), nil
			}
			return out.String(), fmt.Errorf("rcon read response: %w", err)
		}
		if typ != responseValueType {
			continue
		}
		// The sentinel's response marks the end of the real command's output.
		if id == sentinelID {
			return out.String(), nil
		}
		out.Write(body)
	}
}

// authenticate performs the SERVERDATA_AUTH handshake.
func authenticate(conn net.Conn, r *bufio.Reader, password string) error {
	if err := writePacket(conn, commandID, authType, password); err != nil {
		return fmt.Errorf("rcon write auth: %w", err)
	}
	// The server replies with an optional empty RESPONSE_VALUE followed by an
	// AUTH_RESPONSE. A packet ID of -1 in the AUTH_RESPONSE signals bad password.
	for {
		id, typ, _, err := readPacket(r)
		if err != nil {
			return fmt.Errorf("rcon read auth response: %w", err)
		}
		if typ == authResponseType {
			if id == -1 {
				return errAuthFailed
			}
			return nil
		}
		// Ignore the empty RESPONSE_VALUE that precedes the auth response.
	}
}

// writePacket encodes and writes a single Source RCON packet.
func writePacket(w io.Writer, id, typ int32, body string) error {
	var buf bytes.Buffer
	size := int32(len(body) + packetOverhead)
	_ = binary.Write(&buf, binary.LittleEndian, size)
	_ = binary.Write(&buf, binary.LittleEndian, id)
	_ = binary.Write(&buf, binary.LittleEndian, typ)
	buf.WriteString(body)
	buf.WriteByte(0) // body null terminator
	buf.WriteByte(0) // packet null terminator
	_, err := w.Write(buf.Bytes())
	return err
}

// readPacket reads and decodes a single Source RCON packet, returning its ID,
// type and body (with the two trailing null bytes stripped).
func readPacket(r *bufio.Reader) (id, typ int32, body []byte, err error) {
	var size int32
	if err = binary.Read(r, binary.LittleEndian, &size); err != nil {
		return 0, 0, nil, err
	}
	if size < packetOverhead || size > maxPacketSize {
		return 0, 0, nil, fmt.Errorf("rcon: invalid packet size %d", size)
	}
	payload := make([]byte, size)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, 0, nil, err
	}
	id = int32(binary.LittleEndian.Uint32(payload[0:4]))
	typ = int32(binary.LittleEndian.Uint32(payload[4:8]))
	// Body is everything after ID+Type, minus the two trailing null bytes.
	body = payload[8 : len(payload)-2]
	return id, typ, body, nil
}
