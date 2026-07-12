package api

import "testing"

func TestParseStatusPlayers(t *testing.T) {
	// Classic Source/CS2 status player table.
	raw := `hostname: My CS2 Server
version : 1.40.6.1/13961 secure
map     : de_mirage
players : 2 humans, 0 bots (10 max)
# userid name uniqueid connected ping loss state rate adr
# 2 "Alice" STEAM_1:0:12345678 05:23 45 0 active 786432 203.0.113.5:27005
# 3 "Bob Smith" STEAM_1:1:87654321 12:01 78 0 active 786432 198.51.100.9:27005
`
	players := parseStatusPlayers(raw)
	if len(players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(players))
	}

	a := players[0]
	if a.UserID != 2 {
		t.Errorf("player 0 userid: want 2, got %d", a.UserID)
	}
	if a.Name != "Alice" {
		t.Errorf("player 0 name: want Alice, got %q", a.Name)
	}
	if a.SteamID != "STEAM_1:0:12345678" {
		t.Errorf("player 0 steamid: want STEAM_1:0:12345678, got %q", a.SteamID)
	}
	if a.Time != "05:23" {
		t.Errorf("player 0 time: want 05:23, got %q", a.Time)
	}
	if a.Ping != 45 {
		t.Errorf("player 0 ping: want 45, got %d", a.Ping)
	}
	if a.ipInternal != "203.0.113.5" {
		t.Errorf("player 0 ip: want 203.0.113.5, got %q", a.ipInternal)
	}

	if players[1].Name != "Bob Smith" {
		t.Errorf("player 1 name: want 'Bob Smith', got %q", players[1].Name)
	}
}

func TestParseStatusPlayersIgnoresNonPlayerLines(t *testing.T) {
	raw := "hostname: test\nmap: de_dust2\nplayers: 0 humans\n"
	if got := parseStatusPlayers(raw); len(got) != 0 {
		t.Fatalf("expected 0 players, got %d", len(got))
	}
}

func TestParseBannedUsersCfg(t *testing.T) {
	content := `// banned users
banid 0 STEAM_1:0:11111111
banid 30 STEAM_1:1:22222222
// a comment
`
	bans := parseBannedUsersCfg(content)
	if len(bans) != 2 {
		t.Fatalf("expected 2 bans, got %d", len(bans))
	}
	if bans[0].ExpiresAt != "permanent" {
		t.Errorf("ban 0 expires: want permanent, got %q", bans[0].ExpiresAt)
	}
	if bans[0].Source != "cfg" {
		t.Errorf("ban 0 source: want cfg, got %q", bans[0].Source)
	}
	if bans[1].ExpiresAt != "30" {
		t.Errorf("ban 1 expires: want 30, got %q", bans[1].ExpiresAt)
	}
}

func TestParseListID(t *testing.T) {
	raw := `Server bandid list:
STEAM_1:0:11111111 : 0 min
STEAM_1:1:33333333 : 15 min
`
	bans := parseListID(raw)
	if len(bans) != 2 {
		t.Fatalf("expected 2 session bans, got %d", len(bans))
	}
	if bans[0].ExpiresAt != "permanent" {
		t.Errorf("ban 0 expires: want permanent, got %q", bans[0].ExpiresAt)
	}
	if bans[1].ExpiresAt != "15" {
		t.Errorf("ban 1 expires: want 15, got %q", bans[1].ExpiresAt)
	}
}

func TestRemoveBanLine(t *testing.T) {
	content := "banid 0 STEAM_1:0:11111111\nbanid 0 STEAM_1:1:22222222\n"
	out, changed := removeBanLine(content, "STEAM_1:0:11111111")
	if !changed {
		t.Fatal("expected changed=true")
	}
	if want := "banid 0 STEAM_1:1:22222222\n"; out != want {
		t.Errorf("want %q, got %q", want, out)
	}

	_, changed = removeBanLine(content, "STEAM_1:0:99999999")
	if changed {
		t.Error("expected changed=false for absent steamid")
	}
}

func TestIsPrivateIP(t *testing.T) {
	cases := map[string]bool{
		"192.168.1.1": true,
		"10.0.0.5":    true,
		"127.0.0.1":   true,
		"203.0.113.5": false,
		"8.8.8.8":     false,
		"notanip":     true,
	}
	for ip, want := range cases {
		if got := isPrivateIP(ip); got != want {
			t.Errorf("isPrivateIP(%q): want %v, got %v", ip, want, got)
		}
	}
}
