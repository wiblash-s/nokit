package rcon

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// writeTestPacket writes a Source RCON packet from the server side of a test.
func writeTestPacket(w io.Writer, id, typ int32, body string) {
	size := int32(len(body) + packetOverhead)
	_ = binary.Write(w, binary.LittleEndian, size)
	_ = binary.Write(w, binary.LittleEndian, id)
	_ = binary.Write(w, binary.LittleEndian, typ)
	_, _ = w.Write([]byte(body))
	_, _ = w.Write([]byte{0, 0})
}

// startMockRCON starts a mock Source RCON server. On receiving the exec command
// it replies with the packets produced by respond(commandID). It always accepts
// auth. Returns the listen address and a cleanup func.
func startMockRCON(t *testing.T, password string, respond func(cmdID int32) []mockPacket, failAuth bool) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)

		// Read auth packet.
		id, _, _, err := readPacket(r)
		if err != nil {
			return
		}
		if failAuth {
			writeTestPacket(conn, -1, authResponseType, "")
			return
		}
		writeTestPacket(conn, id, authResponseType, "")

		// Read the real command + the sentinel command.
		cmdID, _, _, err := readPacket(r)
		if err != nil {
			return
		}
		// Drain the sentinel command packet.
		_, _, _, _ = readPacket(r)

		for _, p := range respond(cmdID) {
			writeTestPacket(conn, p.id, responseValueType, p.body)
		}
		// Respond to the sentinel to mark end-of-output.
		writeTestPacket(conn, sentinelID, responseValueType, "")
		<-done
	}()
	return ln.Addr().String(), func() {
		close(done)
		ln.Close()
	}
}

type mockPacket struct {
	id   int32
	body string
}

func TestExecuteMultiPacket_ReassemblesMultiplePackets(t *testing.T) {
	// The workshop line lands in the FIRST packet; four more packets of padding
	// follow. A single-packet reader would miss the later data and, worse, desync.
	respond := func(cmdID int32) []mockPacket {
		pkts := []mockPacket{
			{id: cmdID, body: "workshop/3070900859/de_nuke_reloaded\n"},
		}
		for i := 0; i < 4; i++ {
			pkts = append(pkts, mockPacket{id: cmdID, body: strings.Repeat("de_pad_map\n", 100)})
		}
		// A workshop line in a LATER packet too, to prove full reassembly.
		pkts = append(pkts, mockPacket{id: cmdID, body: "workshop/1234567890/cs_office_v2\n"})
		return pkts
	}
	addr, cleanup := startMockRCON(t, "pass", respond, false)
	defer cleanup()

	out, err := executeMultiPacket(addr, "pass", "maps *", 5*time.Second)
	if err != nil {
		t.Fatalf("executeMultiPacket: %v", err)
	}
	if !strings.Contains(out, "3070900859") {
		t.Errorf("missing first-packet workshop line in output")
	}
	if !strings.Contains(out, "1234567890") {
		t.Errorf("missing later-packet workshop line: multi-packet reassembly failed")
	}
	if got := strings.Count(out, "de_pad_map"); got != 400 {
		t.Errorf("expected 400 padding lines, got %d", got)
	}
}

func TestExecuteMultiPacket_SinglePacket(t *testing.T) {
	respond := func(cmdID int32) []mockPacket {
		return []mockPacket{{id: cmdID, body: "workshop/999/de_test\n"}}
	}
	addr, cleanup := startMockRCON(t, "pass", respond, false)
	defer cleanup()

	out, err := executeMultiPacket(addr, "pass", "maps *", 5*time.Second)
	if err != nil {
		t.Fatalf("executeMultiPacket: %v", err)
	}
	if !strings.Contains(out, "de_test") {
		t.Errorf("unexpected output %q", out)
	}
}

func TestExecuteMultiPacket_AuthFailure(t *testing.T) {
	respond := func(cmdID int32) []mockPacket { return nil }
	addr, cleanup := startMockRCON(t, "pass", respond, true)
	defer cleanup()

	_, err := executeMultiPacket(addr, "wrong", "maps *", 5*time.Second)
	if err == nil {
		t.Fatal("expected auth failure error, got nil")
	}
}

func TestExecuteMultiPacket_DialError(t *testing.T) {
	// Nothing listening on this port.
	_, err := executeMultiPacket("127.0.0.1:1", "pass", "maps *", 1*time.Second)
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}
