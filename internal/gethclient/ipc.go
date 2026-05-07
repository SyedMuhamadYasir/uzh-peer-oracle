package gethclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/enodeutil"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Client struct {
	IPCPath     string
	DialContext func(ctx context.Context) (net.Conn, error)
	nextID      uint64
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type NodeInfo struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Enode      string         `json:"enode"`
	ENR        string         `json:"enr"`
	IP         string         `json:"ip"`
	Ports      map[string]int `json:"ports"`
	ListenAddr string         `json:"listenAddr"`
}

type Peer struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Enode     string         `json:"enode"`
	Caps      []string       `json:"caps"`
	Network   PeerNetwork    `json:"network"`
	Protocols map[string]any `json:"protocols"`
}

type PeerNetwork struct {
	LocalAddress  string `json:"localAddress"`
	RemoteAddress string `json:"remoteAddress"`
	Inbound       bool   `json:"inbound"`
	Trusted       bool   `json:"trusted"`
	Static        bool   `json:"static"`
}

type SyncingStatus struct {
	Syncing bool           `json:"syncing"`
	State   map[string]any `json:"state,omitempty"`
}

func New(ipcPath string) *Client {
	return &Client{IPCPath: ipcPath}
}

func NewWithDialer(dial func(ctx context.Context) (net.Conn, error)) *Client {
	return &Client{DialContext: dial}
}

func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	if c.IPCPath == "" && c.DialContext == nil {
		return fmt.Errorf("empty IPC path")
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}
	id := atomic.AddUint64(&c.nextID, 1)
	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return err
	}
	var resp rpcResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: %s", method, resp.Error.Message)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(resp.Result, result)
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if c.DialContext != nil {
		return c.DialContext(ctx)
	}
	d := net.Dialer{}
	return d.DialContext(ctx, "unix", c.IPCPath)
}

func (c *Client) NodeInfo(ctx context.Context) (*NodeInfo, error) {
	var out NodeInfo
	err := c.Call(ctx, "admin_nodeInfo", []any{}, &out)
	return &out, err
}

func (c *Client) AdminPeers(ctx context.Context) ([]Peer, error) {
	var out []Peer
	err := c.Call(ctx, "admin_peers", []any{}, &out)
	return out, err
}

func (c *Client) AddPeer(ctx context.Context, enode string) (bool, error) {
	var ok bool
	err := c.Call(ctx, "admin_addPeer", []any{enode}, &ok)
	return ok, err
}

func (c *Client) RemovePeer(ctx context.Context, enode string) (bool, error) {
	var ok bool
	err := c.Call(ctx, "admin_removePeer", []any{enode}, &ok)
	return ok, err
}

func (c *Client) NetVersion(ctx context.Context) (string, error) {
	var out string
	err := c.Call(ctx, "net_version", []any{}, &out)
	return out, err
}

func (c *Client) ChainID(ctx context.Context) (string, error) {
	var out string
	err := c.Call(ctx, "eth_chainId", []any{}, &out)
	return out, err
}

func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	var out string
	if err := c.Call(ctx, "eth_blockNumber", []any{}, &out); err != nil {
		return 0, err
	}
	return parseHexUint(out)
}

func (c *Client) PeerCount(ctx context.Context) (int, error) {
	var out string
	if err := c.Call(ctx, "net_peerCount", []any{}, &out); err != nil {
		return 0, err
	}
	v, err := parseHexUint(out)
	return int(v), err
}

func (c *Client) ClientVersion(ctx context.Context) (string, error) {
	var out string
	err := c.Call(ctx, "web3_clientVersion", []any{}, &out)
	return out, err
}

func (c *Client) Syncing(ctx context.Context) (*SyncingStatus, error) {
	var raw json.RawMessage
	if err := c.Call(ctx, "eth_syncing", []any{}, &raw); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "false" {
		return &SyncingStatus{Syncing: false}, nil
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return &SyncingStatus{Syncing: true}, nil
	}
	return &SyncingStatus{Syncing: true, State: state}, nil
}

func (c *Client) GenesisHash(ctx context.Context) (string, error) {
	var block map[string]any
	if err := c.Call(ctx, "eth_getBlockByNumber", []any{"0x0", false}, &block); err != nil {
		return "", err
	}
	if h, ok := block["hash"].(string); ok {
		return h, nil
	}
	return "", nil
}

func CollectHeartbeat(ctx context.Context, c *Client, cfg *config.Config) (*oracle.HeartbeatRequest, []Peer, error) {
	info, err := c.NodeInfo(ctx)
	if err != nil {
		return nil, nil, err
	}
	peers, _ := c.AdminPeers(ctx)
	networkID, _ := c.NetVersion(ctx)
	chainID, _ := c.ChainID(ctx)
	blockNumber, _ := c.BlockNumber(ctx)
	peerCount, _ := c.PeerCount(ctx)
	clientVersion, _ := c.ClientVersion(ctx)
	genesisHash, _ := c.GenesisHash(ctx)
	currentIDs := make([]string, 0, len(peers))
	for _, p := range peers {
		if p.ID != "" {
			currentIDs = append(currentIDs, strings.ToLower(p.ID))
		}
	}
	ip := cfg.Geth.PublicIP
	tcpPort := cfg.Geth.P2PTCPPort
	udpPort := cfg.Geth.P2PUDPPort
	nodeID := strings.ToLower(info.ID)
	if parsed, err := enodeutil.Parse(info.Enode); err == nil {
		if nodeID == "" {
			nodeID = parsed.NodeID
		}
		if ip == "" {
			ip = parsed.IP
		}
		if tcpPort == 0 {
			tcpPort = parsed.TCPPort
		}
		if udpPort == 0 {
			udpPort = parsed.UDPPort
		}
	}
	return &oracle.HeartbeatRequest{
		NodeName:       cfg.Agent.NodeName,
		Role:           cfg.Agent.Role,
		NodeID:         nodeID,
		Enode:          info.Enode,
		ENR:            info.ENR,
		IP:             ip,
		TCPPort:        tcpPort,
		UDPPort:        udpPort,
		Zones:          cfg.Zones.CustomZones,
		ZoneHints:      cfg.Zones.Hints,
		NetworkID:      firstNonEmpty(networkID, cfg.Geth.ExpectedNetworkID),
		ChainID:        firstNonEmpty(chainID, cfg.Geth.ExpectedChainID),
		GenesisHash:    firstNonEmpty(genesisHash, cfg.Geth.ExpectedGenesisHash),
		ClientVersion:  firstNonEmpty(clientVersion, info.Name),
		BlockNumber:    blockNumber,
		PeerCount:      peerCount,
		CurrentPeerIDs: currentIDs,
		AgentVersion:   oracle.AgentVersion,
	}, peers, nil
}

func HasEthCapability(p Peer) bool {
	for _, c := range p.Caps {
		if strings.HasPrefix(c, "eth/") {
			return true
		}
	}
	if _, ok := p.Protocols["eth"]; ok {
		return true
	}
	return false
}

func FindPeer(peers []Peer, nodeID string) (Peer, bool) {
	nodeID = strings.ToLower(nodeID)
	for _, p := range peers {
		if strings.ToLower(p.ID) == nodeID {
			return p, true
		}
	}
	return Peer{}, false
}

func parseHexUint(s string) (uint64, error) {
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	if s == "" {
		return 0, nil
	}
	return strconv.ParseUint(s, 16, 64)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
