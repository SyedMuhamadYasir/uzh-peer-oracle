package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Runner struct {
	cfg    *config.Config
	http   *http.Client
	logger *log.Logger
	token  string
}

func New(cfg *config.Config, logger *log.Logger) (*Runner, error) {
	token := cfg.ProbeToken()
	if err := config.RequireToken(token); err != nil {
		return nil, err
	}
	return &Runner{
		cfg:    cfg,
		http:   &http.Client{Timeout: 15 * time.Second},
		logger: logger,
		token:  token,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	r.logger.Info("probe started", map[string]any{"zone": r.cfg.Probe.Zone, "oracle": r.cfg.Probe.OracleURL})
	for {
		if err := r.Cycle(ctx); err != nil {
			r.logger.Warn("probe cycle failed", map[string]any{"err": err.Error()})
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.jittered(config.DurationSeconds(r.cfg.Probe.IntervalSeconds, 30*time.Second))):
		}
	}
}

func (r *Runner) Cycle(ctx context.Context) error {
	nodes, err := r.fetchNodes(ctx)
	if err != nil {
		return err
	}
	report := oracle.ProbeReportRequest{ProbeID: r.cfg.Probe.ProbeID, Zone: r.cfg.Probe.Zone}
	timeout := config.DurationSeconds(r.cfg.Checks.TimeoutSeconds, 3*time.Second)
	for _, n := range nodes {
		if n.NodeID == "" || n.IP == "" || n.TCPPort == 0 {
			continue
		}
		start := time.Now()
		obs := oracle.ProbeObservation{NodeID: n.NodeID, Enode: n.Enode}
		if r.cfg.Checks.TCP {
			addr := net.JoinHostPort(n.IP, strconv.Itoa(n.TCPPort))
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err != nil {
				obs.Error = err.Error()
			} else {
				obs.TCPReachable = true
				obs.LatencyMS = time.Since(start).Milliseconds()
				_ = conn.Close()
			}
		}
		report.Observations = append(report.Observations, obs)
	}
	return r.postReport(ctx, report)
}

func (r *Runner) fetchNodes(ctx context.Context) ([]oracle.Node, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(r.cfg.Probe.OracleURL, "/")+"/v1/debug/nodes", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	res, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("debug nodes failed: %s %s", res.Status, string(b))
	}
	var nodes []oracle.Node
	if err := json.NewDecoder(res.Body).Decode(&nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (r *Runner) postReport(ctx context.Context, report oracle.ProbeReportRequest) error {
	b, _ := json.Marshal(report)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.cfg.Probe.OracleURL, "/")+"/v1/probe-report", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("probe report failed: %s %s", res.Status, string(b))
	}
	r.logger.Info("probe report submitted", map[string]any{"observations": len(report.Observations), "zone": report.Zone})
	return nil
}

func (r *Runner) jittered(base time.Duration) time.Duration {
	j := r.cfg.Probe.JitterPercent
	if j <= 0 {
		return base
	}
	delta := int64(base) * int64(j) / 100
	return base - time.Duration(delta) + time.Duration(rand.Int63n(delta*2+1))
}
