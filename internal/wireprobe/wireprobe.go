package wireprobe

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/enodeutil"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Prober struct {
	cfg config.WireProbeConfig
}

func New(cfg config.WireProbeConfig) *Prober {
	return &Prober{cfg: cfg}
}

func (p *Prober) ProbeEnode(ctx context.Context, raw string) oracle.WireProbeResult {
	return p.probe(ctx, raw, "")
}

func (p *Prober) ProbeENR(ctx context.Context, raw string) oracle.WireProbeResult {
	return p.probe(ctx, "", raw)
}

func (p *Prober) ProbeNode(ctx context.Context, nodeID, enodeRaw, enrRaw string) oracle.WireProbeResult {
	res := p.probe(ctx, enodeRaw, enrRaw)
	if nodeID != "" {
		res.NodeID = nodeID
	}
	return res
}

func (p *Prober) probe(ctx context.Context, enodeRaw, enrRaw string) oracle.WireProbeResult {
	res := oracle.WireProbeResult{
		Enode:           strings.TrimSpace(enodeRaw),
		ENR:             strings.TrimSpace(enrRaw),
		CheckedAt:       time.Now().UTC(),
		ParseStatus:     "not_started",
		TCPStatus:       "not_started",
		Discv4Status:    "not_available",
		Discv5Status:    "not_available",
		RLPxStatus:      "not_available",
		EthStatusStatus: "not_available",
	}
	node, parseTarget, err := parseNode(res.Enode, res.ENR)
	if err != nil {
		res.ParseStatus = "error"
		res.Error = err.Error()
		return res
	}
	res.ParseOK = true
	res.ParseStatus = "ok"
	res.NodeID = node.ID().String()
	res.IP = ""
	if ip := node.IP(); ip != nil {
		res.IP = ip.String()
		res.IPClass = classifyIP(ip)
	}
	res.TCPPort = int(node.TCP())
	res.UDPPort = int(node.UDP())

	if !p.cfg.TCPCheck {
		res.TCPStatus = "disabled"
		return res
	}
	if res.IP == "" || res.TCPPort == 0 {
		res.TCPStatus = "not_available"
		return res
	}
	timeout := config.DurationSeconds(p.cfg.TimeoutSeconds, 5*time.Second)
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(res.IP, fmt.Sprintf("%d", res.TCPPort)))
	if err != nil {
		res.TCPStatus = "error"
		res.Error = appendErr(res.Error, fmt.Sprintf("tcp: %v", err))
	} else {
		res.TCPOK = true
		res.TCPStatus = "ok"
		res.TCPLatencyMS = time.Since(start).Milliseconds()
		_ = conn.SetDeadline(time.Now().Add(timeout))
		_ = conn.Close()
	}

	if p.cfg.DiscoveryCheck {
		if parseTarget == "enode" {
			res.Discv4Status = "not_available"
		} else if parseTarget == "enr" {
			res.Discv5Status = "not_available"
		}
	}
	if p.cfg.RLPxCheck {
		res.RLPxStatus = "not_available"
	}
	if p.cfg.EthStatusCheck {
		res.EthStatusStatus = "not_available"
	}
	return res
}

func Explain(res oracle.WireProbeResult, peerCount, targetPeers int, alreadyConnected bool) string {
	switch {
	case !res.ParseOK:
		return "invalid enode or ENR"
	case res.IPClass == "loopback":
		return "private/loopback advertised IP"
	case res.IPClass == "private":
		return "private/loopback advertised IP"
	case !res.TCPOK && res.TCPStatus == "error":
		return "TCP closed or unreachable"
	case res.TCPOK && res.RLPxStatus == "error":
		return "TCP reachable but RLPx failed"
	case res.RLPxOK && !res.EthStatusOK && res.EthStatusStatus == "error":
		return "RLPx works but eth protocol missing or wrong chain"
	case alreadyConnected:
		return "already connected"
	case peerCount >= targetPeers && targetPeers > 0:
		return "peer slot pressure"
	case res.TCPOK && res.RLPxStatus == "not_available":
		return "TCP reachable; deeper devp2p/RLPx checks not available"
	default:
		return "no strong failure signal recorded"
	}
}

func parseNode(enodeRaw, enrRaw string) (*enode.Node, string, error) {
	if enodeRaw != "" {
		node, err := enode.ParseV4(enodeRaw)
		if err != nil {
			return nil, "", err
		}
		return node, "enode", nil
	}
	if enrRaw != "" {
		node, err := enode.Parse(enode.ValidSchemes, enrRaw)
		if err != nil {
			return nil, "", err
		}
		return node, "enr", nil
	}
	return nil, "", fmt.Errorf("either enode or enr must be provided")
}

func classifyIP(ip net.IP) string {
	if ip == nil {
		return "unknown"
	}
	if ip.IsLoopback() {
		return "loopback"
	}
	if ip.IsPrivate() {
		return "private"
	}
	if enodeutil.IsPublicIP(ip.String()) {
		return "public"
	}
	return "unknown"
}

func appendErr(existing, next string) string {
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	return existing + "; " + next
}
