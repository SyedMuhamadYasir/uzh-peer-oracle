package unit

import (
	"strings"
	"testing"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/seed"
)

func TestSeedParserMetadataAndBadLines(t *testing.T) {
	cfg := config.Defaults()
	input := `
# comment
enode://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@130.60.24.247:30308 # name=hub zone=public role=hub
enr:-fakepublic # name=enr-hub zone=public role=hub
not-a-peer
enode://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb@10.0.0.5:30308 # zone=uzh-vpn role=miner
`
	res := seed.Parse(strings.NewReader(input), cfg)
	if got, want := len(res.Peers), 3; got != want {
		t.Fatalf("peers=%d want=%d errors=%v", got, want, res.Errors)
	}
	if got, want := len(res.Errors), 1; got != want {
		t.Fatalf("errors=%d want=%d", got, want)
	}
	if res.Peers[0].NodeName != "hub" || res.Peers[0].Role != "hub" {
		t.Fatalf("metadata not parsed: %+v", res.Peers[0])
	}
	if res.Peers[1].ENR == "" || res.Peers[1].NodeName != "enr-hub" {
		t.Fatalf("ENR line not parsed: %+v", res.Peers[1])
	}
	if !res.Peers[2].HasZone("uzh-vpn") {
		t.Fatalf("private zone missing: %+v", res.Peers[2].Zones)
	}
}
