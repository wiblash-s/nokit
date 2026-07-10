package loghub

import (
        "io"
        "log/slog"
        "net"
        "testing"
        "time"
)

func discardLogger() *slog.Logger {
        return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestParsePacket(t *testing.T) {
        header := []byte{0xFF, 0xFF, 0xFF, 0xFF}
        mk := func(parts ...[]byte) []byte {
                var out []byte
                out = append(out, header...)
                for _, p := range parts {
                        out = append(out, p...)
                }
                return out
        }

        cases := []struct {
                name   string
                packet []byte
                want   string
                ok     bool
        }{
                {
                        name:   "plain R packet strips L prefix and trailing NUL",
                        packet: mk([]byte("R"), []byte("L 07/10/2026 - 16:16:14: World triggered \"Round_Start\"\x00")),
                        want:   `07/10/2026 - 16:16:14: World triggered "Round_Start"`,
                        ok:     true,
                },
                {
                        name:   "S packet with secret skips to L prefix",
                        packet: mk([]byte("S"), []byte("123456"), []byte("L 07/10/2026 - 16:16:15: host_workshop_map 3070263842\n")),
                        want:   "07/10/2026 - 16:16:15: host_workshop_map 3070263842",
                        ok:     true,
                },
                {
                        name:   "no type byte falls through to body",
                        packet: mk([]byte("L 07/10/2026 - 16:16:16: Loading map\x00")),
                        want:   "07/10/2026 - 16:16:16: Loading map",
                        ok:     true,
                },
                {
                        name:   "too short",
                        packet: []byte{0xFF, 0xFF},
                        want:   "",
                        ok:     false,
                },
                {
                        name:   "bad header",
                        packet: []byte{0x00, 0x01, 0x02, 0x03, 'R', 'x'},
                        want:   "",
                        ok:     false,
                },
                {
                        name:   "empty body after trim",
                        packet: mk([]byte("R"), []byte("L \x00")),
                        want:   "",
                        ok:     false,
                },
        }

        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        got, ok := ParsePacket(tc.packet)
                        if ok != tc.ok {
                                t.Fatalf("ok = %v, want %v (line=%q)", ok, tc.ok, got)
                        }
                        if got != tc.want {
                                t.Fatalf("line = %q, want %q", got, tc.want)
                        }
                })
        }
}

func TestSubscribeBroadcastUnsubscribe(t *testing.T) {
        h := New(discardLogger())

        id1, ch1 := h.Subscribe()
        _, ch2 := h.Subscribe()

        h.broadcast("hello")

        for _, ch := range []<-chan string{ch1, ch2} {
                select {
                case got := <-ch:
                        if got != "hello" {
                                t.Fatalf("got %q, want hello", got)
                        }
                case <-time.After(time.Second):
                        t.Fatal("timed out waiting for broadcast")
                }
        }

        // After unsubscribe, ch1 is closed and no longer receives.
        h.Unsubscribe(id1)
        if _, open := <-ch1; open {
                t.Fatal("expected ch1 to be closed after unsubscribe")
        }

        h.broadcast("second")
        select {
        case got := <-ch2:
                if got != "second" {
                        t.Fatalf("got %q, want second", got)
                }
        case <-time.After(time.Second):
                t.Fatal("timed out waiting for second broadcast")
        }
}

func TestSlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
        h := New(discardLogger())
        h.Subscribe() // never drained

        done := make(chan struct{})
        go func() {
                for i := 0; i < subBuffer+100; i++ {
                        h.broadcast("x")
                }
                close(done)
        }()

        select {
        case <-done:
        case <-time.After(2 * time.Second):
                t.Fatal("broadcast blocked on a slow subscriber")
        }
}

func TestListenReceivesRealUDPPacket(t *testing.T) {
        h := New(discardLogger())
        // Bind an ephemeral port by asking the OS, then reuse it.
        tmp, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
        if err != nil {
                t.Fatalf("probe bind: %v", err)
        }
        port := tmp.LocalAddr().(*net.UDPAddr).Port
        tmp.Close()

        if err := h.Listen(port); err != nil {
                t.Fatalf("listen: %v", err)
        }
        defer h.Close()

        _, ch := h.Subscribe()

        conn, err := net.Dial("udp", net.JoinHostPort("127.0.0.1", itoa(port)))
        if err != nil {
                t.Fatalf("dial: %v", err)
        }
        defer conn.Close()

        packet := append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'R'},
                []byte("L 07/10/2026 - 16:16:14: Map loaded\x00")...)
        if _, err := conn.Write(packet); err != nil {
                t.Fatalf("write: %v", err)
        }

        select {
        case got := <-ch:
                want := "07/10/2026 - 16:16:14: Map loaded"
                if got != want {
                        t.Fatalf("got %q, want %q", got, want)
                }
        case <-time.After(2 * time.Second):
                t.Fatal("timed out waiting for UDP packet")
        }
}

// TestParseHTTPBody verifies that plain-text HTTP log bodies (delivered by CS2
// via `logaddress_add_http`) are split into clean lines matching the UDP output.
func TestParseHTTPBody(t *testing.T) {
        cases := []struct {
                name string
                body string
                want []string
        }{
                {
                        name: "single line with L prefix and newline",
                        body: "L 07/10/2026 - 16:16:14: World triggered \"Round_Start\"\n",
                        want: []string{`07/10/2026 - 16:16:14: World triggered "Round_Start"`},
                },
                {
                        name: "multiple lines batched in one body",
                        body: "L 07/10/2026 - 16:16:15: host_workshop_map 3070263842\nL 07/10/2026 - 16:16:16: Loading map\n",
                        want: []string{
                                "07/10/2026 - 16:16:15: host_workshop_map 3070263842",
                                "07/10/2026 - 16:16:16: Loading map",
                        },
                },
                {
                        name: "blank and whitespace lines are skipped",
                        body: "L 07/10/2026 - 16:16:17: line one\n\n   \nL 07/10/2026 - 16:16:18: line two\n",
                        want: []string{
                                "07/10/2026 - 16:16:17: line one",
                                "07/10/2026 - 16:16:18: line two",
                        },
                },
                {
                        name: "line without L prefix or trailing newline",
                        body: "07/10/2026 - 16:16:19: no prefix",
                        want: []string{"07/10/2026 - 16:16:19: no prefix"},
                },
                {
                        name: "empty body yields nothing",
                        body: "",
                        want: nil,
                },
        }

        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        got := ParseHTTPBody([]byte(tc.body))
                        if len(got) != len(tc.want) {
                                t.Fatalf("got %d lines %v, want %d %v", len(got), got, len(tc.want), tc.want)
                        }
                        for i := range got {
                                if got[i] != tc.want[i] {
                                        t.Errorf("line %d = %q, want %q", i, got[i], tc.want[i])
                                }
                        }
                })
        }
}

// TestPublish verifies that Publish fans a line out to subscribers (the path the
// HTTP listener uses), and is a no-op after the hub is closed.
func TestPublish(t *testing.T) {
        h := New(discardLogger())
        _, ch := h.Subscribe()

        h.Publish("07/10/2026 - 16:16:14: host_workshop_map 3070263842")
        select {
        case got := <-ch:
                want := "07/10/2026 - 16:16:14: host_workshop_map 3070263842"
                if got != want {
                        t.Fatalf("got %q, want %q", got, want)
                }
        case <-time.After(time.Second):
                t.Fatal("timed out waiting for published line")
        }

        // After Close, Publish must not panic (channels are closed) and simply drops.
        h.Close()
        h.Publish("this should be dropped, not panic")
}

// itoa avoids importing strconv just for the test port.
func itoa(n int) string {
        if n == 0 {
                return "0"
        }
        var b [6]byte
        i := len(b)
        for n > 0 {
                i--
                b[i] = byte('0' + n%10)
                n /= 10
        }
        return string(b[i:])
}
