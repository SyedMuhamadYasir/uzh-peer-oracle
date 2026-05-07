package matcher

import (
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/enodeutil"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Request struct {
	Requester      *oracle.Node
	EffectiveZones []string
	CurrentPeerIDs []string
	LatestProbes   map[string]oracle.WireProbeResult
	Limit          int
	Now            time.Time
}

func Select(req Request, all []*oracle.Node, edges []oracle.ReachabilityEdge, cfg *config.Config) []oracle.PeerRecommendation {
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = cfg.Selection.MaxPeersReturned
	}
	if limit <= 0 {
		limit = 32
	}
	allowed := set(req.EffectiveZones)
	current := set(req.CurrentPeerIDs)
	if req.Requester != nil {
		for _, p := range req.Requester.CurrentPeerIDs {
			current[p] = true
		}
	}
	edgeScore := map[string]float64{}
	reciprocalBoost := map[string]float64{}
	for _, e := range edges {
		if e.ToID == "" {
			continue
		}
		if req.Requester != nil && e.FromID == req.Requester.NodeID {
			if e.ConfidenceScore > edgeScore[e.ToID] {
				edgeScore[e.ToID] = e.ConfidenceScore
			}
		}
		if allowed[e.FromZone] && e.ConfidenceScore > edgeScore[e.ToID] {
			edgeScore[e.ToID] = e.ConfidenceScore
		}
		if req.Requester != nil && e.ToID == req.Requester.NodeID && e.FailureCount > e.SuccessCount && !strings.HasPrefix(e.FromID, "probe:") && !strings.HasPrefix(e.FromID, "zone:") {
			reciprocalBoost[e.FromID] += 3
		}
	}
	type candidate struct {
		node       *oracle.Node
		confidence float64
		score      float64
	}
	var candidates []candidate
	for _, n := range all {
		if n == nil || n.Banned || n.Enode == "" {
			continue
		}
		if req.Requester != nil && n.NodeID == req.Requester.NodeID {
			continue
		}
		if current[n.NodeID] {
			continue
		}
		if cfg.Network.ExpectedNetworkID != "" && n.NetworkID != "" && n.NetworkID != cfg.Network.ExpectedNetworkID {
			continue
		}
		if cfg.Network.ExpectedChainID != "" && n.ChainID != "" && !sameChainID(n.ChainID, cfg.Network.ExpectedChainID) {
			continue
		}
		if cfg.Network.ExpectedGenesisHash != "" && n.GenesisHash != "" && !strings.EqualFold(n.GenesisHash, cfg.Network.ExpectedGenesisHash) {
			continue
		}
		if stale(n, now, cfg.Verification.DeadAfterSeconds) {
			continue
		}
		if !zoneAllowed(n, allowed, cfg.Zones.DefaultPublicZone) {
			continue
		}
		conf := edgeScore[n.NodeID]
		if conf == 0 {
			conf = baselineConfidence(n, allowed, cfg.Zones.DefaultPublicZone)
		}
		score := float64(n.Score) + conf*10
		if cfg.Selection.PreferHubs && n.Role == "hub" {
			score += 8
		}
		if zoneOverlap(n.Zones, req.EffectiveZones) {
			score += 4
		}
		if !n.LastSeen.IsZero() {
			age := now.Sub(n.LastSeen)
			if age < 30*time.Second {
				score += 3
			} else if age < 2*time.Minute {
				score += 1
			}
		}
		if probe, ok := req.LatestProbes[n.NodeID]; ok {
			score += probeScore(probe)
		}
		score += reciprocalBoost[n.NodeID]
		score += fairJitter(req.Requester, n, now)
		candidates = append(candidates, candidate{node: n, confidence: conf, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})
	var out []oracle.PeerRecommendation
	subnetCount := map[string]int{}
	roleCount := map[string]int{}
	for _, c := range candidates {
		if len(out) >= limit {
			break
		}
		if cfg.Selection.DiversifySubnets && c.node.IP != "" {
			key := enodeutil.SubnetKey(c.node.IP)
			if subnetCount[key] >= 3 {
				continue
			}
			subnetCount[key]++
		}
		if roleCount[c.node.Role] >= limit/2 && c.node.Role != "" && c.node.Role != "hub" {
			continue
		}
		roleCount[c.node.Role]++
		out = append(out, oracle.PeerRecommendation{
			NodeID:                 c.node.NodeID,
			NodeName:               c.node.NodeName,
			Role:                   c.node.Role,
			Enode:                  c.node.Enode,
			ENR:                    c.node.ENR,
			IP:                     c.node.IP,
			TCPPort:                c.node.TCPPort,
			UDPPort:                c.node.UDPPort,
			Zones:                  c.node.Zones,
			Score:                  int(c.score),
			ReachabilityConfidence: c.confidence,
			LastSeen:               c.node.LastSeen,
		})
	}
	return out
}

func zoneAllowed(n *oracle.Node, allowed map[string]bool, publicZone string) bool {
	for _, z := range n.Zones {
		if allowed[z] {
			return true
		}
	}
	return n.HasZone(publicZone) && allowed[publicZone]
}

func baselineConfidence(n *oracle.Node, allowed map[string]bool, publicZone string) float64 {
	if n.HasZone(publicZone) && allowed[publicZone] {
		return 0.72
	}
	for _, z := range n.Zones {
		if allowed[z] {
			return 0.64
		}
	}
	return 0.1
}

func stale(n *oracle.Node, now time.Time, deadAfter int) bool {
	if n.IsSeed && n.LastSeen.IsZero() {
		return false
	}
	if deadAfter <= 0 || n.LastSeen.IsZero() {
		return false
	}
	return now.Sub(n.LastSeen) > time.Duration(deadAfter)*time.Second
}

func sameChainID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func zoneOverlap(a, b []string) bool {
	s := set(a)
	for _, z := range b {
		if s[z] {
			return true
		}
	}
	return false
}

func set(in []string) map[string]bool {
	out := map[string]bool{}
	for _, x := range in {
		x = strings.TrimSpace(x)
		if x != "" {
			out[x] = true
		}
	}
	return out
}

func fairJitter(requester, target *oracle.Node, now time.Time) float64 {
	h := fnv.New32a()
	if requester != nil {
		_, _ = h.Write([]byte(requester.NodeID))
	}
	_, _ = h.Write([]byte(target.NodeID))
	_, _ = h.Write([]byte(now.UTC().Format("200601021504")))
	return float64(h.Sum32()%1000) / 1000.0
}

func probeScore(probe oracle.WireProbeResult) float64 {
	score := 0.0
	if probe.TCPOK {
		score += 8
	} else if probe.TCPStatus == "error" {
		score -= 6
	}
	if probe.RLPxOK {
		score += 12
	} else if probe.RLPxStatus == "error" {
		score -= 8
	}
	if probe.EthStatusOK {
		score += 16
	} else if probe.EthStatusStatus == "error" {
		score -= 10
	}
	if probe.IPClass == "private" || probe.IPClass == "loopback" {
		score -= 10
	}
	return score
}
