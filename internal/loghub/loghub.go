// Package loghub ingests CS2 / Source-engine server logs delivered over UDP
// (the classic `logaddress_add` mechanism) and fans them out to any number of
// live subscribers (e.g. the SSE Live Logs panel).
//
// A CS2 dedicated server, once told `logaddress_add <host:port>; log on`,
// emits every console/log line as a UDP datagram to that address. Each
// datagram is a Source "log packet":
//
//	0xFF 0xFF 0xFF 0xFF            4-byte connectionless header
//	<type byte>                   'R' (0x52) plain, or 'S' (0x53) with sv_logsecret
//	[secret]                      only present for 'S' packets (ASCII digits)
//	L 07/10/2026 - 16:16:14: ...  the log body, prefixed with "L "
//	0x00 / 0x0A                   trailing NUL and/or newline
//
// The hub binds a single UDP socket, parses each datagram into a clean line,
// and broadcasts it to all current subscribers. Subscribers use buffered
// channels; a slow subscriber drops lines rather than blocking the reader.
package loghub

import (
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
)

// subBuffer is the per-subscriber channel capacity. If a subscriber can't keep
// up, lines beyond this buffer are dropped for that subscriber only.
const subBuffer = 512

// readBuffer is the max UDP datagram size we read. Source log lines are short,
// but we allow room for long workshop URLs / plugin output.
const readBuffer = 64 * 1024

// Hub receives UDP log datagrams and fans out parsed lines to subscribers.
type Hub struct {
	logger *slog.Logger

	mu     sync.Mutex
	subs   map[int]chan string
	nextID int
	conn   *net.UDPConn
	closed bool
}

// New creates an unstarted Hub. Call Listen to bind the UDP socket.
func New(logger *slog.Logger) *Hub {
	return &Hub{
		logger: logger,
		subs:   make(map[int]chan string),
	}
}

// Listen binds the UDP socket on the given port (all interfaces) and starts the
// background read loop. It returns an error if the port cannot be bound; the
// caller may choose to log-and-continue so the rest of the panel still works.
func (h *Hub) Listen(port int) error {
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("loghub: bind udp :%d: %w", port, err)
	}

	h.mu.Lock()
	h.conn = conn
	h.mu.Unlock()

	h.logger.Info("loghub listening for CS2 logs", "port", port, "proto", "udp")
	go h.readLoop(conn)
	return nil
}

// readLoop reads datagrams until the socket is closed.
func (h *Hub) readLoop(conn *net.UDPConn) {
	buf := make([]byte, readBuffer)
	var received, parsed uint64
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			h.mu.Lock()
			closed := h.closed
			h.mu.Unlock()
			if closed {
				return
			}
			// Transient read error: log and keep going.
			h.logger.Warn("loghub udp read", "error", err)
			continue
		}

		received++
		line, ok := ParsePacket(buf[:n])
		if ok {
			parsed++
		}

		// Diagnostics: announce the very first datagram (proves CS2 is
		// reaching us and from which address), and warn if a datagram
		// arrives but cannot be parsed as a Source log packet.
		if received == 1 {
			h.logger.Info("loghub received first UDP datagram",
				"from", src.String(), "bytes", n, "parsed", ok, "line", line)
		}
		if !ok {
			h.logger.Warn("loghub could not parse datagram",
				"from", src.String(), "bytes", n, "head", previewBytes(buf[:n]))
			continue
		}

		h.broadcast(line)
	}
}

// previewBytes returns a short, printable hex/ascii preview of a datagram head
// for troubleshooting unparseable packets.
func previewBytes(b []byte) string {
	const max = 24
	if len(b) > max {
		b = b[:max]
	}
	return fmt.Sprintf("% x", b)
}

// Subscribe registers a new subscriber and returns its id plus a receive-only
// channel of log lines. Call Unsubscribe with the id when done.
func (h *Hub) Subscribe() (int, <-chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.nextID
	h.nextID++
	ch := make(chan string, subBuffer)
	h.subs[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (h *Hub) Unsubscribe(id int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(ch)
	}
}

// broadcast sends a line to every subscriber without blocking. A subscriber
// whose buffer is full drops the line.
func (h *Hub) broadcast(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subs {
		select {
		case ch <- line:
		default:
			// Subscriber is behind; drop this line for them only.
		}
	}
}

// Close shuts down the UDP socket and closes all subscriber channels.
func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	var err error
	if h.conn != nil {
		err = h.conn.Close()
	}
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
	return err
}

// ParsePacket extracts a clean log line from a raw Source UDP log datagram.
// It returns the line and true on success, or ("", false) if the datagram is
// not a recognisable log packet.
func ParsePacket(b []byte) (string, bool) {
	// Need at least the 4-byte header + type byte.
	if len(b) < 5 {
		return "", false
	}
	// Connectionless header: 0xFFFFFFFF.
	if b[0] != 0xFF || b[1] != 0xFF || b[2] != 0xFF || b[3] != 0xFF {
		return "", false
	}

	body := b[4:]
	switch body[0] {
	case 'R': // 0x52 - plain log packet
		body = body[1:]
	case 'S': // 0x53 - packet carries sv_logsecret; skip up to the "L " prefix
		if idx := bytes.Index(body, []byte("L ")); idx >= 0 {
			body = body[idx:]
		} else {
			body = body[1:]
		}
	default:
		// Some servers send the body without an explicit R/S type byte.
		// Fall through and treat the whole remainder as the body.
	}

	// Trim the trailing NUL/newline first (but not the space that follows the
	// "L " marker), then drop the leading Source "L " marker for cleaner
	// display; the timestamp that follows is preserved.
	line := strings.TrimRight(string(body), "\x00\r\n")
	line = strings.TrimPrefix(line, "L ")
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	return line, true
}
