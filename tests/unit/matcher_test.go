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

func TestMatcherBoostsWireProbeHealthyPeers(t *testing.T) {
	cfg := config.Defaults()
	now := time.Now().UTC()
	nodes := []*oracle.Node{
		{NodeID: "self", Enode: "enode://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@203.0.113.1:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "plain", NodeName: "plain", Enode: "enode://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb@203.0.113.2:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "healthy", NodeName: "healthy", Enode: "enode://cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc@203.0.113.3:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
	}
	recs := matcher.Select(matcher.Request{
		Requester:      nodes[0],
		EffectiveZones: []string{"public"},
		LatestProbes: map[string]oracle.WireProbeResult{
			"healthy": {NodeID: "healthy", ParseOK: true, TCPOK: true, TCPStatus: "ok", RLPxOK: true, RLPxStatus: "ok"},
			"plain":   {NodeID: "plain", ParseOK: true, TCPOK: false, TCPStatus: "error"},
		},
		Limit: 10,
		Now:   now,
	}, nodes, nil, cfg)
	if len(recs) < 2 {
		t.Fatalf("expected at least two recs: %+v", recs)
	}
	if recs[0].NodeID != "healthy" {
		t.Fatalf("expected healthy peer first, got %+v", recs)
	}
}

func TestMatcherGeneratesReciprocalRecommendationAfterDirectionalFailure(t *testing.T) {
	cfg := config.Defaults()
	now := time.Now().UTC()
	nodes := []*oracle.Node{
		{NodeID: "a", NodeName: "A", Enode: "enode://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@203.0.113.1:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "b", NodeName: "B", Enode: "enode://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb@203.0.113.2:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "c", NodeName: "C", Enode: "enode://cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc@203.0.113.3:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
	}
	edges := []oracle.ReachabilityEdge{
		{FromID: "a", FromZone: "public", ToID: "b", FailureCount: 2, SuccessCount: 0, LastFailure: now, ConfidenceScore: 0.1},
	}
	recs := matcher.Select(matcher.Request{
		Requester:      nodes[1],
		EffectiveZones: []string{"public"},
		Limit:          10,
		Now:            now,
	}, nodes, edges, cfg)
	if len(recs) == 0 || recs[0].NodeID != "a" {
		t.Fatalf("expected reciprocal recommendation for A, got %+v", recs)
	}
}
