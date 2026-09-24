package main

import (
	"flag"
	"log"
	"os"
)

const version = "2.5.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("rex-node v%s — Remote EXecution Protocol daemon", version)
	log.Printf("Protocol: RXP/2.5")

	configPath := flag.String("config", getEnvOrDefault("REX_CONFIG", "/etc/rex/config.yaml"), "Path to config file")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[rex] config error: %v", err)
	}

	allowlist, err := LoadAllowlist(cfg.AllowlistPath)
	if err != nil {
		log.Fatalf("[rex] allowlist error: %v", err)
	}

	server := NewServer(cfg, allowlist)
	if err := server.Start(); err != nil {
		log.Fatalf("[rex] server error: %v", err)
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
