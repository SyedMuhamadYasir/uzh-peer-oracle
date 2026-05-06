package unit

import (
	"testing"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/matcher"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

func TestMatcherDoesNotLeakPrivatePeersToPublicRequester(t *testing.T) {
	cfg := config.Defaults()
	now := time.Now().UTC()
	nodes := []*oracle.Node{
		{NodeID: "self", Enode: "enode://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@203.0.113.1:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "pub", Enode: "enode://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb@203.0.113.2:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "vpn", Enode: "enode://cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc@10.0.0.2:30308", Zones: []string{"uzh-vpn"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
	}
	recs := matcher.Select(matcher.Request{
		Requester:      nodes[0],
		EffectiveZones: []string{"public"},
		Limit:          10,
		Now:            now,
	}, nodes, nil, cfg)
	for _, r := range recs {
		if r.NodeID == "vpn" {
			t.Fatalf("private peer leaked to public requester: %+v", r)
		}
	}
	if len(recs) != 1 || recs[0].NodeID != "pub" {
		t.Fatalf("unexpected recs: %+v", recs)
	}
}
