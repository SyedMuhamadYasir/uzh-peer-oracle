package enodeutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Parsed struct {
	Raw     string
	NodeID  string
	Host    string
	IP      string
	TCPPort int
	UDPPort int
}

func Parse(raw string) (*Parsed, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "enode://") {
		return nil, fmt.Errorf("not an enode URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.User == nil {
		return nil, fmt.Errorf("missing node public key")
	}
	nodeID := strings.ToLower(u.User.Username())
	if len(nodeID) < 32 || !isHex(nodeID) {
		return nil, fmt.Errorf("invalid node id %q", nodeID)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("missing host")
	}
	tcpPort := 0
	if u.Port() != "" {
		tcpPort, err = strconv.Atoi(u.Port())
		if err != nil {
			return nil, fmt.Errorf("invalid TCP port: %w", err)
		}
	}
	udpPort := tcpPort
	if discport := u.Query().Get("discport"); discport != "" {
		udpPort, err = strconv.Atoi(discport)
		if err != nil {
			return nil, fmt.Errorf("invalid discport: %w", err)
		}
	}
	ip := ""
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		ip = parsedIP.String()
	}
	return &Parsed{Raw: raw, NodeID: nodeID, Host: host, IP: ip, TCPPort: tcpPort, UDPPort: udpPort}, nil
}

func NodeIDFromENR(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return "enr-" + hex.EncodeToString(sum[:16])
}

func IsPrivateOrLoopback(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func IsPublicIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

func SubnetKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
