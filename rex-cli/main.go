package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	Version = "2.5.0"
	Banner  = `
 ██████╗ ███████╗██╗  ██╗
 ██╔══██╗██╔════╝╚██╗██╔╝
 ██████╔╝█████╗   ╚███╔╝ 
 ██╔══██╗██╔══╝   ██╔██╗ 
 ██║  ██║███████╗██╔╝ ██╗
 ╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝  Connect (RXP/2.5 — WarpGate)
`
)

func printUsage() {
	fmt.Printf("%s\nVersion: %s (100%% Native Go Client)\n\n", Banner, Version)
	fmt.Println("Client Usage:")
	fmt.Println("  rex connect <host:port> -t <token>   Attach interactive terminal (SSH-like)")
	fmt.Println("  rex exec    <host:port> -t <token> \"<cmd>\"   Fast live command execution")
	fmt.Println("  rex info    <host:port> -t <token>   Display remote system hardware metrics")
	fmt.Println()
	fmt.Println("Server & Daemon Management:")
	fmt.Println("  rex info                             Display local node config & token")
	fmt.Println("  rex update                           Update REX CLI and Node daemon to latest version")
	fmt.Println("  rex status                           Check local daemon service status")
	fmt.Println("  rex restart                          Restart local daemon")
	fmt.Println("  rex logs                             Stream live systemd logs")
	fmt.Println("  rex mode {autonomous|review|allowlist} Configure security policy mode")
	fmt.Println("  rex uninstall                        Remove REX Node daemon from server")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --token, -t     Authentication bearer token")
	fmt.Println("  --tls           Enable TLS 1.3 encryption (Default: true)")
	fmt.Println("  --insecure, -k  Skip certificate verification (Self-signed)")
	fmt.Println("  --raw, -r       Output raw payload without terminal framing")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  rex connect 5.202.5.134:7444 --token mysecrettoken")
	fmt.Println("  rex exec 5.202.5.134:7444 -t mytoken \"uptime\"")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version", "-v", "--version":
		fmt.Printf("rex version %s (RXP/2.5 WarpGate)\n", Version)
		return

	case "help", "-h", "--help":
		printUsage()
		return

	case "connect":
		runConnect(os.Args[2:])

	case "exec":
		runExec(os.Args[2:])

	case "info":
		runInfo(os.Args[2:])

	case "update":
		runUpdate()

	case "status":
		runStatus()

	case "restart":
		runRestart()

	case "start":
		runStart()

	case "stop":
		runStop()

	case "logs":
		runLogs(os.Args[2:])

	case "mode":
		runMode(os.Args[2:])

	case "uninstall":
		runUninstall()

	default:
		// If first arg looks like host, host:port, @user, or user@host, treat as shortcut for 'rex connect'
		if strings.Contains(command, ":") || strings.Contains(command, "@") || strings.Count(command, ".") >= 2 {
			runConnect(os.Args[1:])
			return
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func parseTargetAddress(input string) string {
	target := strings.TrimSpace(input)

	// Format 1: @root 5.202.5.134 -> handled by checking caller args, but if target has @prefix:
	if strings.HasPrefix(target, "@") {
		// e.g. @root: stripped
		target = strings.TrimPrefix(target, "@")
		if idx := strings.Index(target, " "); idx != -1 {
			target = target[idx+1:]
		}
	}

	// Format 2: root@5.202.5.134 or @5.202.5.134
	if strings.Contains(target, "@") {
		parts := strings.SplitN(target, "@", 2)
		target = parts[1]
	}

	target = strings.TrimSpace(target)
	if !strings.Contains(target, ":") {
		target = target + ":7444"
	}
	return target
}

func splitFlagsAndPosArgs(args []string) ([]string, []string) {
	var flagArgs []string
	var posArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-t" || a == "-token" || a == "--token" {
			flagArgs = append(flagArgs, a)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			posArgs = append(posArgs, a)
		}
	}
	return flagArgs, posArgs
}

func runConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	token := fs.String("token", "", "Authentication token")
	fs.StringVar(token, "t", "", "Authentication token (shorthand)")
	useTLS := fs.Bool("tls", true, "Use TLS 1.3 encryption")
	insecure := fs.Bool("insecure", true, "Skip TLS cert verification")
	fs.BoolVar(insecure, "k", true, "Skip TLS cert verification (shorthand)")

	flagArgs, posArgs := splitFlagsAndPosArgs(args)

	if err := fs.Parse(flagArgs); err != nil {
		os.Exit(1)
	}

	if len(posArgs) < 1 {
		fmt.Fprintln(os.Stderr, "Error: Missing <host:port> address")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  rex @root 5.202.5.134")
		fmt.Fprintln(os.Stderr, "  rex root@5.202.5.134")
		fmt.Fprintln(os.Stderr, "  rex connect 5.202.5.134:7444")
		os.Exit(1)
	}

	var rawAddr string
	// Check if user passed: rex @root 5.202.5.134 (two positional arguments)
	if len(posArgs) >= 2 && (strings.HasPrefix(posArgs[0], "@") || posArgs[0] == "root") {
		rawAddr = posArgs[1]
	} else {
		rawAddr = posArgs[0]
	}

	addr := parseTargetAddress(rawAddr)

	if *token == "" {
		*token = os.Getenv("REX_TOKEN")
	}

	// Check local saved token cache (~/.rex/tokens.json)
	if *token == "" {
		*token = getStoredToken(addr)
		if *token != "" {
			fmt.Printf("🔑 Using saved token for %s\n", addr)
		}
	}

	// If token is still empty, prompt the user securely (like SSH password prompt)!
	if *token == "" {
		enteredToken, err := ReadPassword(fmt.Sprintf("🔑 Enter REX Token for %s: ", addr))
		if err != nil || strings.TrimSpace(enteredToken) == "" {
			fmt.Fprintln(os.Stderr, "\nError: Authentication token required.")
			os.Exit(1)
		}
		*token = strings.TrimSpace(enteredToken)
		// Save for future connections
		saveStoredToken(addr, *token)
	}

	fmt.Printf("⚡ Connecting to %s (TLS 1.3: %v)...\n", addr, *useTLS)
	client, err := Dial(addr, *token, *useTLS, *insecure)
	if err != nil {
		// If TLS failed, give helpful diagnostic
		fmt.Fprintf(os.Stderr, "❌ Connection failed: %v\n", err)
		if *useTLS {
			fmt.Fprintln(os.Stderr, "💡 Tip: If your remote server runs plaintext TCP, pass --tls=false")
		}
		os.Exit(1)
	}
	defer client.Close()

	// Launch interactive terminal (StreamID: 1)
	if err := InteractiveTerminal(client, 1); err != nil {
		fmt.Fprintf(os.Stderr, "\nTerminal session closed: %v\n", err)
	} else {
		fmt.Println("\nConnection closed gracefully.")
	}
}

func runExec(args []string) {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	token := fs.String("token", "", "Authentication token")
	fs.StringVar(token, "t", "", "Authentication token (shorthand)")
	useTLS := fs.Bool("tls", true, "Use TLS 1.3 encryption")
	insecure := fs.Bool("insecure", true, "Skip TLS cert verification")
	fs.BoolVar(insecure, "k", true, "Skip TLS cert verification (shorthand)")
	raw := fs.Bool("raw", false, "Output raw payload without terminal framing")
	fs.BoolVar(raw, "r", false, "Output raw payload (shorthand)")

	flagArgs, posArgs := splitFlagsAndPosArgs(args)

	if err := fs.Parse(flagArgs); err != nil {
		os.Exit(1)
	}

	if len(posArgs) < 2 {
		fmt.Fprintln(os.Stderr, "❌ Error: Missing server address or command")
		fmt.Fprintln(os.Stderr, "💡 REX Agent Quickstart:")
		fmt.Fprintln(os.Stderr, "   • Execute command: rex exec <IP>:7444 -t <TOKEN> \"<command>\"")
		fmt.Fprintln(os.Stderr, "   • Full terminal:   rex connect <IP>:7444 -t <TOKEN>")
		os.Exit(1)
	}

	addr := parseTargetAddress(posArgs[0])
	remoteCmd := posArgs[1]

	if *token == "" {
		*token = os.Getenv("REX_TOKEN")
	}
	if *token == "" {
		*token = getStoredToken(addr)
	}
	if *token == "" {
		enteredToken, err := ReadPassword(fmt.Sprintf("🔑 Enter REX Token for %s: ", addr))
		if err != nil || strings.TrimSpace(enteredToken) == "" {
			fmt.Fprintln(os.Stderr, "\n❌ Error: Authentication token required.")
			fmt.Fprintln(os.Stderr, "💡 Pass token via flag: rex exec <IP>:7444 -t <TOKEN> \"<command>\"")
			fmt.Fprintln(os.Stderr, "   Or set environment variable: export REX_TOKEN=<TOKEN>")
			os.Exit(1)
		}
		*token = strings.TrimSpace(enteredToken)
		saveStoredToken(addr, *token)
	}

	execStart := time.Now()

	client, err := Dial(addr, *token, *useTLS, *insecure)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection failed to %s: %v\n", addr, err)
		fmt.Fprintln(os.Stderr, "💡 REX Agent Troubleshooting Guide:")
		fmt.Fprintln(os.Stderr, "   1. Target Port: Connect to TCP Port 7444 (e.g. 5.202.5.134:7444)")
		fmt.Fprintln(os.Stderr, "   2. TLS 1.3: Enabled by default. If remote daemon runs plaintext, pass --tls=false")
		fmt.Fprintln(os.Stderr, "   3. Token: Verify token via -t <TOKEN> or export REX_TOKEN=<TOKEN>")
		os.Exit(1)
	}
	defer client.Close()

	if !*raw {
		fmt.Printf("┌── ⚡ REX [%s]\n", addr)
		fmt.Printf("│ ❯ %s\n", remoteCmd)
		fmt.Println("├── OUTPUT ──────────────────────────────────────────────────────────")
	}

	// Register stream 2 for Fast Exec
	ch := client.RegisterStream(2)
	defer client.UnregisterStream(2)

	// Send OpFastExec directly (Sub-millisecond latency, zero PTY overhead)
	if err := client.Send(OpFastExec, 2, []byte(remoteCmd)); err != nil {
		if !*raw {
			fmt.Fprintf(os.Stderr, "❌ FastExec failed: %v\n", err)
			fmt.Printf("└── [Elapsed: %v | Status: FAILED] ─────────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
		} else {
			fmt.Fprintf(os.Stderr, "❌ FastExec failed: %v\n", err)
		}
		os.Exit(1)
	}

	// Wait for execution result frame; if server runs older daemon without FastExec, fallback to PTY
	select {
	case resFrame := <-ch:
		if resFrame == nil {
			if !*raw {
				fmt.Printf("└── [Elapsed: %v | Status: DISCONNECTED] ───────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
			}
			return
		}
		if resFrame.Opcode == OpError {
			if !*raw {
				fmt.Fprintf(os.Stderr, "❌ %s\n", string(resFrame.Payload))
				fmt.Printf("└── [Elapsed: %v | Status: FAILED] ─────────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
			} else {
				fmt.Fprintf(os.Stderr, "❌ %s\n", string(resFrame.Payload))
			}
			os.Exit(1)
		}
		if *raw {
			os.Stdout.Write(resFrame.Payload)
		} else {
			out := string(resFrame.Payload)
			if len(out) > 0 {
				if !strings.HasSuffix(out, "\n") {
					out += "\n"
				}
				os.Stdout.WriteString(out)
			}
			fmt.Printf("└── [Elapsed: %v | Status: OK] ─────────────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
		}
		return

	case <-time.After(1000 * time.Millisecond):
		// Fallback for older daemons: execute via PTY stream
		chPTY := client.RegisterStream(3)
		defer client.UnregisterStream(3)

		if err := client.Send(OpPTYSpawn, 3, []byte{0x00, 0x50, 0x00, 0x18}); err != nil {
			if !*raw {
				fmt.Fprintf(os.Stderr, "❌ Spawn failed: %v\n", err)
				fmt.Printf("└── [Elapsed: %v | Status: FAILED] ─────────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
			} else {
				fmt.Fprintf(os.Stderr, "❌ Spawn failed: %v\n", err)
			}
			os.Exit(1)
		}

		ack := <-chPTY
		if ack == nil || ack.Opcode == OpError {
			errMsg := "Session spawn rejected by server"
			if ack != nil && len(ack.Payload) > 0 {
				errMsg = string(ack.Payload)
			}
			if !*raw {
				fmt.Fprintf(os.Stderr, "❌ Server rejected: %s\n", errMsg)
				fmt.Printf("└── [Elapsed: %v | Status: FAILED] ─────────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
			} else {
				fmt.Fprintf(os.Stderr, "❌ Server rejected: %s\n", errMsg)
			}
			os.Exit(1)
		}

		// Send command with trailing newline and exit
		payload := []byte(remoteCmd + "\nexit\n")
		_ = client.Send(OpPTYData, 3, payload)

		// Stream stdout live to terminal until closed or idle timeout
		for {
			select {
			case frame, ok := <-chPTY:
				if !ok || frame == nil || frame.Opcode == OpPTYClose {
					if !*raw {
						fmt.Printf("\n└── [Elapsed: %v | Status: PTY OK] ─────────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
					}
					return
				}
				if frame.Opcode == OpPTYData {
					_, _ = os.Stdout.Write(frame.Payload)
				}
			case <-time.After(1200 * time.Millisecond):
				if !*raw {
					fmt.Printf("\n└── [Elapsed: %v | Status: PTY TIMEOUT] ────────────────────────────\n", time.Since(execStart).Round(time.Millisecond))
				}
				return
			}
		}
	}
}

func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	token := fs.String("token", "", "Authentication token")
	fs.StringVar(token, "t", "", "Authentication token (shorthand)")
	useTLS := fs.Bool("tls", true, "Use TLS 1.3 encryption")
	insecure := fs.Bool("insecure", true, "Skip TLS cert verification")

	flagArgs, posArgs := splitFlagsAndPosArgs(args)

	if err := fs.Parse(flagArgs); err != nil {
		os.Exit(1)
	}

	if len(posArgs) < 1 {
		configPath := "/etc/rex/config.yaml"
		if data, err := os.ReadFile(configPath); err == nil {
			fmt.Println("==========================================")
			fmt.Println(" 📍 REX Node Local Configuration")
			fmt.Println("==========================================")
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "token:") || strings.HasPrefix(line, "mode:") || strings.HasPrefix(line, "port:") || strings.HasPrefix(line, "tcp_port:") {
					fmt.Println(" " + line)
				}
			}
			fmt.Println("==========================================")
			return
		}
		fmt.Fprintln(os.Stderr, "Error: Missing <host:port> address")
		fmt.Fprintln(os.Stderr, "Usage: rex info <host:port> -t <token>")
		os.Exit(1)
	}

	addr := posArgs[0]
	if !strings.Contains(addr, ":") {
		addr = addr + ":7444"
	}

	if *token == "" {
		*token = os.Getenv("REX_TOKEN")
	}

	client, err := Dial(addr, *token, *useTLS, *insecure)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	ch := client.RegisterStream(3)
	defer client.UnregisterStream(3)

	_ = client.Send(OpNativeSysInfo, 3, nil)
	frame := <-ch
	if frame != nil {
		fmt.Println(string(frame.Payload))
	}
}

func runStart() {
	cmd := exec.Command("systemctl", "start", "rex-node")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Start failed: %v\n", err)
	} else {
		fmt.Println("✅ REX Node daemon started.")
	}
}

func runStop() {
	cmd := exec.Command("systemctl", "stop", "rex-node")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Stop failed: %v\n", err)
	} else {
		fmt.Println("🛑 REX Node daemon stopped.")
	}
}

func runRestart() {
	fmt.Println("🔄 Restarting REX Node daemon...")
	cmd := exec.Command("systemctl", "restart", "rex-node")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Restart failed: %v\n", err)
	} else {
		fmt.Println("✅ REX Node daemon restarted successfully.")
	}
}

func runStatus() {
	cmd := exec.Command("systemctl", "status", "rex-node", "--no-pager")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func runLogs(args []string) {
	cmdArgs := append([]string{"-u", "rex-node", "-f", "-n", "50"}, args...)
	cmd := exec.Command("journalctl", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}

func runUninstall() {
	fmt.Println("⚠️ Removing REX Node from server...")
	_ = exec.Command("systemctl", "stop", "rex-node").Run()
	_ = exec.Command("systemctl", "disable", "rex-node").Run()
	_ = os.Remove("/etc/systemd/system/rex-node.service")
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = os.Remove("/usr/local/bin/rex-node")
	_ = os.RemoveAll("/etc/rex")
	fmt.Println("🗑️ REX Node daemon has been completely uninstalled and removed.")
}

func runMode(args []string) {
	configPath := "/etc/rex/config.yaml"
	if len(args) == 0 {
		data, err := os.ReadFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error reading %s: %v\n", configPath, err)
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "mode:") {
				fmt.Println(strings.TrimSpace(line))
				return
			}
		}
		fmt.Println("mode: autonomous")
		return
	}
	newMode := args[0]
	if newMode != "autonomous" && newMode != "review" && newMode != "allowlist" {
		fmt.Fprintln(os.Stderr, "❌ Invalid mode. Choose: autonomous | review | allowlist")
		return
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error reading %s: %v\n", configPath, err)
		return
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "mode:") {
			lines[i] = "mode: " + newMode
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, "mode: "+newMode)
	}
	if err := os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to update config: %v\n", err)
		return
	}
	_ = exec.Command("systemctl", "restart", "rex-node").Run()
	fmt.Printf("✅ Security mode set to: %s (daemon restarted)\n", newMode)
}

func runUpdate() {
	fmt.Println("🔄 Updating REX components to latest version...")
	arch := runtime.GOARCH
	var cliBin, nodeBin string
	switch arch {
	case "amd64":
		cliBin = "rex-linux-amd64"
		nodeBin = "rex-node-linux-amd64"
	case "arm64":
		cliBin = "rex-linux-arm64"
		nodeBin = "rex-node-linux-arm64"
	default:
		fmt.Printf("❌ Unsupported Architecture: %s\n", arch)
		return
	}

	// 1. Update rex CLI
	cliPath, err := os.Executable()
	if err != nil || strings.Contains(cliPath, "go-build") {
		cliPath = "/usr/local/bin/rex"
	}
	cliUrl := fmt.Sprintf("https://github.com/dalroot/rex/releases/download/v2.5.0/%s", cliBin)
	fmt.Printf("📦 Updating REX CLI (%s)...\n", cliPath)
	cmd := exec.Command("curl", "-L", "-f", "-s", "-S", cliUrl, "-o", cliPath+".tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to download CLI update: %s (%v)\n", string(out), err)
	} else {
		_ = os.Chmod(cliPath+".tmp", 0755)
		_ = os.Rename(cliPath+".tmp", cliPath)
		fmt.Println("✅ REX CLI updated successfully.")
	}

	// 2. If rex-node exists, update node daemon as well!
	if _, err := os.Stat("/usr/local/bin/rex-node"); err == nil {
		fmt.Println("📦 Updating REX Node daemon (/usr/local/bin/rex-node)...")
		nodeUrl := fmt.Sprintf("https://github.com/dalroot/rex/releases/download/v2.5.0/%s", nodeBin)
		cmdNode := exec.Command("curl", "-L", "-f", "-s", "-S", nodeUrl, "-o", "/usr/local/bin/rex-node.tmp")
		if out, err := cmdNode.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to download Node daemon update: %s (%v)\n", string(out), err)
		} else {
			_ = os.Chmod("/usr/local/bin/rex-node.tmp", 0755)
			_ = os.Rename("/usr/local/bin/rex-node.tmp", "/usr/local/bin/rex-node")
			_ = exec.Command("systemctl", "restart", "rex-node").Run()
			fmt.Println("✅ REX Node daemon updated and restarted successfully.")
		}
	}
}

func getTokensFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".rex")
	_ = os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "tokens.json")
}

func getStoredToken(addr string) string {
	file := getTokensFilePath()
	if file == "" {
		return ""
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var store map[string]string
	if err := json.Unmarshal(data, &store); err != nil {
		return ""
	}
	return store[addr]
}

func saveStoredToken(addr, token string) {
	file := getTokensFilePath()
	if file == "" {
		return
	}
	store := make(map[string]string)
	if data, err := os.ReadFile(file); err == nil {
		_ = json.Unmarshal(data, &store)
	}
	store[addr] = token
	data, err := json.MarshalIndent(store, "", "  ")
	if err == nil {
		_ = os.WriteFile(file, data, 0600)
	}
}
