package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Auth         AuthConfig         `yaml:"auth"`
	Network      NetworkConfig      `yaml:"network"`
	WireProbe    WireProbeConfig    `yaml:"wire_probe"`
	Zones        ZonesConfig        `yaml:"zones"`
	Seed         SeedConfig         `yaml:"seed"`
	Verification VerificationConfig `yaml:"verification"`
	Selection    SelectionConfig    `yaml:"selection"`
	Snapshot     SnapshotConfig     `yaml:"snapshot"`
	Storage      StorageConfig      `yaml:"storage"`
	Metrics      MetricsConfig      `yaml:"metrics"`
	Agent        AgentConfig        `yaml:"agent"`
	Geth         GethConfig         `yaml:"geth"`
	Safety       SafetyConfig       `yaml:"safety"`
	LocalState   LocalStateConfig   `yaml:"local_state"`
	Probe        ProbeConfig        `yaml:"probe"`
	Checks       ProbeChecksConfig  `yaml:"checks"`
}

type ServerConfig struct {
	Listen    string `yaml:"listen"`
	PublicURL string `yaml:"public_url"`
}

type AuthConfig struct {
	Mode           string        `yaml:"mode"`
	SharedTokenEnv string        `yaml:"shared_token_env"`
	Tokens         []TokenConfig `yaml:"tokens"`
}

type TokenConfig struct {
	Name         string   `yaml:"name"`
	TokenEnv     string   `yaml:"token_env"`
	Token        string   `yaml:"token"`
	AllowedZones []string `yaml:"allowed_zones"`
	Role         string   `yaml:"role"`
}

type NetworkConfig struct {
	ExpectedNetworkID   string `yaml:"expected_network_id"`
	ExpectedChainID     string `yaml:"expected_chain_id"`
	ExpectedGenesisHash string `yaml:"expected_genesis_hash"`
	DefaultTCPPort      int    `yaml:"default_tcp_port"`
	DefaultUDPPort      int    `yaml:"default_udp_port"`
}

type WireProbeConfig struct {
	Enabled        bool `yaml:"enabled"`
	TCPCheck       bool `yaml:"tcp_check"`
	DiscoveryCheck bool `yaml:"discovery_check"`
	RLPxCheck      bool `yaml:"rlpx_check"`
	EthStatusCheck bool `yaml:"eth_status_check"`
	TimeoutSeconds int  `yaml:"timeout_seconds"`
	MaxParallel    int  `yaml:"max_parallel"`
}

type ZonesConfig struct {
	DefaultPublicZone string   `yaml:"default_public_zone"`
	UniversityCIDRs   []string `yaml:"university_cidrs"`
	PrivateZones      []string `yaml:"private_zones"`
	CustomZones       []string `yaml:"custom_zones"`
	Hints             []string `yaml:"hints"`
	AutoDetect        bool     `yaml:"auto_detect"`
}

type SeedConfig struct {
	Path                  string `yaml:"path"`
	ReloadIntervalSeconds int    `yaml:"reload_interval_seconds"`
}

type VerificationConfig struct {
	RequireCorrectNetworkID           bool `yaml:"require_correct_network_id"`
	RequireCorrectChainID             bool `yaml:"require_correct_chain_id"`
	RequireGenesisHashIfConfigured    bool `yaml:"require_genesis_hash_if_configured"`
	RequireTCPReachableForPublic      bool `yaml:"require_tcp_reachable_for_public"`
	AllowPrivateIPsOnlyInPrivateZones bool `yaml:"allow_private_ips_only_in_private_zones"`
	UseDevP2PIfAvailable              bool `yaml:"use_devp2p_if_available"`
	TCPTimeoutSeconds                 int  `yaml:"tcp_timeout_seconds"`
	StaleAfterSeconds                 int  `yaml:"stale_after_seconds"`
	DeadAfterSeconds                  int  `yaml:"dead_after_seconds"`
}

type SelectionConfig struct {
	DefaultTargetPeers int  `yaml:"default_target_peers"`
	MaxPeersReturned   int  `yaml:"max_peers_returned"`
	PreferHubs         bool `yaml:"prefer_hubs"`
	DiversifyIPs       bool `yaml:"diversify_ips"`
	DiversifySubnets   bool `yaml:"diversify_subnets"`
	RotateResults      bool `yaml:"rotate_results"`
}

type SnapshotConfig struct {
	Enabled          bool   `yaml:"enabled"`
	PrivateKeyPath   string `yaml:"private_key_path"`
	PublishPublicKey bool   `yaml:"publish_public_key"`
}

type StorageConfig struct {
	Driver      string `yaml:"driver"`
	SQLitePath  string `yaml:"sqlite_path"`
	PostgresDSN string `yaml:"postgres_dsn"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
}

type AgentConfig struct {
	NodeName                   string `yaml:"node_name"`
	Role                       string `yaml:"role"`
	OracleURL                  string `yaml:"oracle_url"`
	TokenEnv                   string `yaml:"token_env"`
	HeartbeatIntervalSeconds   int    `yaml:"heartbeat_interval_seconds"`
	PeerRefreshIntervalSeconds int    `yaml:"peer_refresh_interval_seconds"`
	JitterPercent              int    `yaml:"jitter_percent"`
	TargetPeers                int    `yaml:"target_peers"`
	MaxManagedPeers            int    `yaml:"max_managed_peers"`
	DryRun                     bool   `yaml:"dry_run"`
}

type GethConfig struct {
	IPCPath             string `yaml:"ipc_path"`
	ExpectedNetworkID   string `yaml:"expected_network_id"`
	ExpectedChainID     string `yaml:"expected_chain_id"`
	ExpectedGenesisHash string `yaml:"expected_genesis_hash"`
	PublicIP            string `yaml:"public_ip"`
	P2PTCPPort          int    `yaml:"p2p_tcp_port"`
	P2PUDPPort          int    `yaml:"p2p_udp_port"`
}

type SafetyConfig struct {
	NeverRemoveUnmanagedPeers         bool   `yaml:"never_remove_unmanaged_peers"`
	RequirePublicEnodeIPForPublicZone bool   `yaml:"require_public_enode_ip_for_public_zone"`
	WarnIfEnodeIPMismatch             bool   `yaml:"warn_if_enode_ip_mismatch"`
	VerifySignedSnapshots             bool   `yaml:"verify_signed_snapshots"`
	OracleSnapshotPublicKey           string `yaml:"oracle_snapshot_public_key"`
}

type LocalStateConfig struct {
	Path string `yaml:"path"`
}

type ProbeConfig struct {
	ProbeID         string `yaml:"probe_id"`
	Zone            string `yaml:"zone"`
	OracleURL       string `yaml:"oracle_url"`
	TokenEnv        string `yaml:"token_env"`
	IntervalSeconds int    `yaml:"interval_seconds"`
	JitterPercent   int    `yaml:"jitter_percent"`
}

type ProbeChecksConfig struct {
	TCP            bool   `yaml:"tcp"`
	UDPHint        bool   `yaml:"udp_hint"`
	DevP2PV4       bool   `yaml:"devp2p_v4"`
	DevP2PV5       bool   `yaml:"devp2p_v5"`
	DevP2PPath     string `yaml:"devp2p_path"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

func Load(path string) (*Config, error) {
	cfg := Defaults()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, cfg); err != nil {
		return nil, err
	}
	cfg.Normalize()
	return cfg, nil
}

func Defaults() *Config {
	cfg := &Config{}
	cfg.Server.Listen = "127.0.0.1:8787"
	cfg.Auth.Mode = "shared_token"
	cfg.Auth.SharedTokenEnv = "UZH_PEER_ORACLE_TOKEN"
	cfg.Network.ExpectedNetworkID = "702"
	cfg.Network.ExpectedChainID = "0x2be"
	cfg.Network.DefaultTCPPort = 30308
	cfg.Network.DefaultUDPPort = 30308
	cfg.WireProbe.Enabled = true
	cfg.WireProbe.TCPCheck = true
	cfg.WireProbe.DiscoveryCheck = true
	cfg.WireProbe.RLPxCheck = true
	cfg.WireProbe.EthStatusCheck = false
	cfg.WireProbe.TimeoutSeconds = 5
	cfg.WireProbe.MaxParallel = 16
	cfg.Zones.DefaultPublicZone = "public"
	cfg.Zones.UniversityCIDRs = []string{"130.60.0.0/16", "157.173.0.0/16"}
	cfg.Zones.PrivateZones = []string{"uzh-vpn", "uzh-campus", "lab-a"}
	cfg.Zones.AutoDetect = true
	cfg.Seed.ReloadIntervalSeconds = 30
	cfg.Verification.RequireCorrectNetworkID = true
	cfg.Verification.RequireCorrectChainID = true
	cfg.Verification.RequireGenesisHashIfConfigured = true
	cfg.Verification.RequireTCPReachableForPublic = true
	cfg.Verification.AllowPrivateIPsOnlyInPrivateZones = true
	cfg.Verification.UseDevP2PIfAvailable = true
	cfg.Verification.TCPTimeoutSeconds = 2
	cfg.Verification.StaleAfterSeconds = 120
	cfg.Verification.DeadAfterSeconds = 600
	cfg.Selection.DefaultTargetPeers = 12
	cfg.Selection.MaxPeersReturned = 32
	cfg.Selection.PreferHubs = true
	cfg.Selection.DiversifyIPs = true
	cfg.Selection.DiversifySubnets = true
	cfg.Selection.RotateResults = true
	cfg.Snapshot.Enabled = false
	cfg.Snapshot.PrivateKeyPath = "./data/snapshot_ed25519.key"
	cfg.Snapshot.PublishPublicKey = true
	cfg.Storage.Driver = "sqlite"
	cfg.Storage.SQLitePath = "./data/oracle.db"
	cfg.Metrics.Enabled = true
	cfg.Metrics.Listen = "127.0.0.1:9797"
	cfg.Agent.Role = "normal"
	cfg.Agent.TokenEnv = "UZH_PEER_ORACLE_TOKEN"
	cfg.Agent.HeartbeatIntervalSeconds = 15
	cfg.Agent.PeerRefreshIntervalSeconds = 20
	cfg.Agent.JitterPercent = 30
	cfg.Agent.TargetPeers = 12
	cfg.Agent.MaxManagedPeers = 24
	cfg.Geth.IPCPath = "/home/yasir/uzhethereum/.uzhethereum/geth.ipc"
	cfg.Geth.ExpectedNetworkID = "702"
	cfg.Geth.ExpectedChainID = "0x2be"
	cfg.Geth.P2PTCPPort = 30308
	cfg.Geth.P2PUDPPort = 30308
	cfg.Safety.NeverRemoveUnmanagedPeers = true
	cfg.Safety.RequirePublicEnodeIPForPublicZone = true
	cfg.Safety.WarnIfEnodeIPMismatch = true
	cfg.Safety.VerifySignedSnapshots = true
	cfg.LocalState.Path = "./data/agent-state.json"
	cfg.Probe.TokenEnv = "UZH_PEER_ORACLE_TOKEN"
	cfg.Probe.IntervalSeconds = 30
	cfg.Probe.JitterPercent = 30
	cfg.Checks.TCP = true
	cfg.Checks.UDPHint = true
	cfg.Checks.DevP2PV4 = true
	cfg.Checks.DevP2PV5 = true
	cfg.Checks.DevP2PPath = "devp2p"
	cfg.Checks.TimeoutSeconds = 3
	return cfg
}

func (c *Config) Normalize() {
	if c.Server.Listen == "" {
		c.Server.Listen = "127.0.0.1:8787"
	}
	if c.Auth.SharedTokenEnv == "" {
		c.Auth.SharedTokenEnv = "UZH_PEER_ORACLE_TOKEN"
	}
	if c.Network.DefaultTCPPort == 0 {
		c.Network.DefaultTCPPort = 30308
	}
	if c.Network.DefaultUDPPort == 0 {
		c.Network.DefaultUDPPort = 30308
	}
	if c.WireProbe.TimeoutSeconds == 0 {
		c.WireProbe.TimeoutSeconds = 5
	}
	if c.WireProbe.MaxParallel == 0 {
		c.WireProbe.MaxParallel = 16
	}
	if c.Zones.DefaultPublicZone == "" {
		c.Zones.DefaultPublicZone = "public"
	}
	if c.Verification.StaleAfterSeconds == 0 {
		c.Verification.StaleAfterSeconds = 120
	}
	if c.Verification.DeadAfterSeconds == 0 {
		c.Verification.DeadAfterSeconds = 600
	}
	if c.Selection.DefaultTargetPeers == 0 {
		c.Selection.DefaultTargetPeers = 12
	}
	if c.Selection.MaxPeersReturned == 0 {
		c.Selection.MaxPeersReturned = 32
	}
	if c.Storage.Driver == "" {
		c.Storage.Driver = "sqlite"
	}
	if c.Storage.SQLitePath == "" {
		c.Storage.SQLitePath = "/var/lib/uzh-peer-oracle/oracle.db"
	}
	if c.Agent.TokenEnv == "" {
		c.Agent.TokenEnv = "UZH_PEER_ORACLE_TOKEN"
	}
	if c.Agent.HeartbeatIntervalSeconds == 0 {
		c.Agent.HeartbeatIntervalSeconds = 15
	}
	if c.Agent.PeerRefreshIntervalSeconds == 0 {
		c.Agent.PeerRefreshIntervalSeconds = 20
	}
	if c.Agent.TargetPeers == 0 {
		c.Agent.TargetPeers = 12
	}
	if c.Agent.MaxManagedPeers == 0 {
		c.Agent.MaxManagedPeers = 24
	}
	if c.Geth.P2PTCPPort == 0 {
		c.Geth.P2PTCPPort = 30308
	}
	if c.Geth.P2PUDPPort == 0 {
		c.Geth.P2PUDPPort = 30308
	}
	if c.Probe.TokenEnv == "" {
		c.Probe.TokenEnv = "UZH_PEER_ORACLE_TOKEN"
	}
	if c.Probe.IntervalSeconds == 0 {
		c.Probe.IntervalSeconds = 30
	}
	if c.Checks.TimeoutSeconds == 0 {
		c.Checks.TimeoutSeconds = 3
	}
}

func (c *Config) SharedToken() string {
	if c.Auth.SharedTokenEnv != "" {
		return os.Getenv(c.Auth.SharedTokenEnv)
	}
	return ""
}

func (c *Config) AgentToken() string {
	if c.Agent.TokenEnv != "" {
		return os.Getenv(c.Agent.TokenEnv)
	}
	return ""
}

func (c *Config) ProbeToken() string {
	if c.Probe.TokenEnv != "" {
		return os.Getenv(c.Probe.TokenEnv)
	}
	return ""
}

func (c *Config) AllZones() []string {
	seen := map[string]bool{}
	var zones []string
	add := func(z string) {
		z = strings.TrimSpace(z)
		if z == "" || seen[z] {
			return
		}
		seen[z] = true
		zones = append(zones, z)
	}
	add(c.Zones.DefaultPublicZone)
	for _, z := range c.Zones.PrivateZones {
		add(z)
	}
	for _, z := range c.Zones.CustomZones {
		add(z)
	}
	return zones
}

func DurationSeconds(v int, fallback time.Duration) time.Duration {
	if v <= 0 {
		return fallback
	}
	return time.Duration(v) * time.Second
}

func RequireToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("empty bearer token; export UZH_PEER_ORACLE_TOKEN or configure a token")
	}
	return nil
}
