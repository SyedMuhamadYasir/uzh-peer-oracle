package seed

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/enodeutil"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Result struct {
	Peers  []*oracle.Node `json:"peers"`
	Errors []LineError    `json:"errors"`
}

type LineError struct {
	Line  int    `json:"line"`
	Error string `json:"error"`
	Text  string `json:"text"`
}

func ParseFile(path string, cfg *config.Config) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f, cfg), nil
}

func Parse(r io.Reader, cfg *config.Config) *Result {
	res := &Result{}
	byID := map[string]*oracle.Node{}
	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		peerText, meta := splitLine(raw)
		if strings.TrimSpace(peerText) == "" {
			continue
		}
		n, err := parsePeerLine(peerText, meta, cfg)
		if err != nil {
			res.Errors = append(res.Errors, LineError{Line: lineNo, Error: err.Error(), Text: raw})
			continue
		}
		if existing := byID[n.NodeID]; existing != nil {
			merge(existing, n)
			continue
		}
		byID[n.NodeID] = n
		res.Peers = append(res.Peers, n)
	}
	if err := scanner.Err(); err != nil {
		res.Errors = append(res.Errors, LineError{Line: lineNo, Error: err.Error()})
	}
	return res
}

func splitLine(line string) (string, map[string]string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil
	}
	idx := strings.Index(line, "#")
	if idx < 0 {
		return strings.TrimSpace(line), map[string]string{}
	}
	body := strings.TrimSpace(line[:idx])
	comment := strings.TrimSpace(line[idx+1:])
	return body, parseMeta(comment)
}

func parseMeta(comment string) map[string]string {
	meta := map[string]string{}
	for _, part := range strings.Fields(comment) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return meta
}

func parsePeerLine(peerText string, meta map[string]string, cfg *config.Config) (*oracle.Node, error) {
	now := time.Now().UTC()
	n := &oracle.Node{
		NodeName:     meta["name"],
		Role:         firstNonEmpty(meta["role"], "normal"),
		Zones:        nil,
		NetworkID:    cfg.Network.ExpectedNetworkID,
		ChainID:      cfg.Network.ExpectedChainID,
		GenesisHash:  cfg.Network.ExpectedGenesisHash,
		Status:       "seed",
		Score:        1,
		IsSeed:       true,
		Source:       "seed",
		FirstSeen:    now,
		LastSeen:     now,
		AgentVersion: oracle.AgentVersion,
	}
	if strings.HasPrefix(peerText, "enode://") {
		parsed, err := enodeutil.Parse(peerText)
		if err != nil {
			return nil, err
		}
		n.NodeID = parsed.NodeID
		n.Enode = peerText
		n.IP = parsed.IP
		n.TCPPort = firstInt(parsed.TCPPort, cfg.Network.DefaultTCPPort)
		n.UDPPort = firstInt(parsed.UDPPort, cfg.Network.DefaultUDPPort)
	} else if strings.HasPrefix(peerText, "enr:") {
		n.NodeID = enodeutil.NodeIDFromENR(peerText)
		n.ENR = peerText
		n.TCPPort = cfg.Network.DefaultTCPPort
		n.UDPPort = cfg.Network.DefaultUDPPort
	} else {
		return nil, fmt.Errorf("expected enode:// or enr:")
	}
	if meta["network_id"] != "" {
		n.NetworkID = meta["network_id"]
	}
	if meta["chain_id"] != "" {
		n.ChainID = meta["chain_id"]
	}
	if meta["genesis_hash"] != "" {
		n.GenesisHash = meta["genesis_hash"]
	}
	if meta["tcp_port"] != "" {
		if p, err := strconv.Atoi(meta["tcp_port"]); err == nil {
			n.TCPPort = p
		}
	}
	if meta["udp_port"] != "" {
		if p, err := strconv.Atoi(meta["udp_port"]); err == nil {
			n.UDPPort = p
		}
	}
	if meta["zone"] != "" {
		n.Zones = append(n.Zones, strings.Split(meta["zone"], ",")...)
	}
	if meta["zones"] != "" {
		n.Zones = append(n.Zones, strings.Split(meta["zones"], ",")...)
	}
	if len(n.Zones) == 0 {
		n.Zones = inferZones(n, cfg)
	}
	n.Zones = uniqueZones(n.Zones)
	return n, nil
}

func inferZones(n *oracle.Node, cfg *config.Config) []string {
	if n.IP == "" {
		return []string{"unknown"}
	}
	if enodeutil.IsPublicIP(n.IP) {
		return []string{cfg.Zones.DefaultPublicZone}
	}
	if enodeutil.IsPrivateOrLoopback(n.IP) {
		return []string{"private", "unknown"}
	}
	return []string{"unknown"}
}

func uniqueZones(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, z := range in {
		z = strings.TrimSpace(z)
		if z == "" || seen[z] {
			continue
		}
		seen[z] = true
		out = append(out, z)
	}
	return out
}

func merge(dst, src *oracle.Node) {
	if dst.Enode == "" {
		dst.Enode = src.Enode
	}
	if dst.ENR == "" {
		dst.ENR = src.ENR
	}
	if dst.IP == "" {
		dst.IP = src.IP
	}
	if dst.NodeName == "" {
		dst.NodeName = src.NodeName
	}
	dst.Zones = uniqueZones(append(dst.Zones, src.Zones...))
	if src.TCPPort != 0 {
		dst.TCPPort = src.TCPPort
	}
	if src.UDPPort != 0 {
		dst.UDPPort = src.UDPPort
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstInt(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}
