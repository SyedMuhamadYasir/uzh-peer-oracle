package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const repoRoot = "/mnt/d/Download/Academic Stuff/PhD/Codex Projects/uzh-peer-oracle"

func TestContaboWrapperScriptsExist(t *testing.T) {
	files := []string{
		"scripts/contabo_oracle_up.sh",
		"scripts/contabo_doctor.sh",
		"scripts/contabo_agent_once.sh",
		"scripts/contabo_agent_loop.sh",
		"scripts/contabo_measure_before.sh",
		"scripts/contabo_measure_after.sh",
		"scripts/contabo_smoke.sh",
		"scripts/contabo_tmux_up.sh",
		"scripts/contabo_tmux_attach.sh",
		"scripts/contabo_bootnodes_toml.sh",
	}
	for _, rel := range files {
		if _, err := os.Stat(filepath.Join(repoRoot, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestStudentModeDocExists(t *testing.T) {
	if _, err := os.Stat(filepath.Join(repoRoot, "docs/STUDENT_MODE.md")); err != nil {
		t.Fatalf("missing student mode doc: %v", err)
	}
}

func TestMakefileContaboTargetsExist(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	targets := []string{
		"contabo-doctor:",
		"contabo-oracle:",
		"contabo-once:",
		"contabo-loop:",
		"contabo-before:",
		"contabo-after:",
		"contabo-smoke:",
		"contabo-tmux:",
		"contabo-bootnodes:",
	}
	for _, target := range targets {
		if !strings.Contains(text, target) {
			t.Fatalf("missing make target %s", target)
		}
	}
}

func TestCommandSurfaceStillPresent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, "cmd/uzh-peer-oracle/main.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	needles := []string{
		`case "server":`,
		`case "agent":`,
		`case "contabo-doctor":`,
		`case "wire-probe":`,
		`case "diagnose-peer":`,
		`case "explain-peer-failure":`,
		`fs.Bool("once", false, "run one heartbeat/peering cycle and exit")`,
	}
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing command surface marker %q", needle)
		}
	}
}
