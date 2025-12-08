package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

const version = "v1.0.0"

var (
	nodeID      string
	configDir   = "/etc/caddy"
	caddyfile   = "/etc/caddy/Caddyfile"
	controlURL  = "https://control.gtw.uvrs.xyz" // control dashboard
	heartbeatOK = false
)

func main() {
	flag.StringVar(&nodeID, "node-id", os.Getenv("NODE_ID"), "Node ID (e.g. svr-gtw-nd1.uvrs.xyz)")
	flag.Parse()

	if nodeID == "" {
		log.Fatal("NODE_ID not set. Use --node-id or NODE_ID env var")
	}

	log.Printf("gtw-agent %s starting — node: %s", version, nodeID)

	// Initial pull & reload
	gitPull()
	validateAndReload()

	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		gitPull()
		validateAndReload()
		sendHeartbeat()
	}
}

func gitPull() {
	cmd := exec.Command("git", "-C", configDir, "pull", "--ff-only")

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Git pull failed: %v\n%s", err, string(output))
		return
	}

	if bytes.Contains(output, []byte("Already up to date")) {
		log.Println("Config already up to date")
		return
	}

	log.Printf("Config updated via git pull:\n%s", string(output))
}

func validateAndReload() {
	if err := exec.Command("caddy", "validate", "--config", caddyfile).Run(); err != nil {
		log.Printf("Validation failed: %v", err)
		heartbeatOK = false
		return
	}
	if err := exec.Command("caddy", "reload", "--config", caddyfile).Run(); err != nil {
		log.Printf("Reload failed: %v", err)
		heartbeatOK = false
		return
	}
	log.Println("Caddy reloaded successfully")
	heartbeatOK = true
}

func sendHeartbeat() {
	sha, _ := exec.Command("git", "-C", configDir, "rev-parse", "HEAD").Output()
	payload := map[string]interface{}{
		"node_id":         nodeID,
		"git_sha":         string(bytes.TrimSpace(sha)),
		"agent_version":   version,
		"caddy_version":   getCaddyVersion(),
		"last_reload_ok":  heartbeatOK,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
	jsonBody, _ := json.Marshal(payload)

	// Silently fail — control plane not required yet
	http.Post(controlURL+"/api/heartbeat", "application/json", bytes.NewReader(jsonBody))
}

func getCaddyVersion() string {
	out, _ := exec.Command("caddy", "version").Output()
	return string(bytes.TrimSpace(out))
}
