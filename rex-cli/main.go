package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	fmt.Println("Usage:")
	fmt.Println("  rex connect <host:port> --token <token> [--tls] [--insecure]   Attach interactive terminal (SSH-like)")
	fmt.Println("  rex exec    <host:port> --token <token> \"<command>\"           Fast live command execution")
	fmt.Println("  rex info    <host:port> --token <token>                      Display remote system hardware metrics")
	fmt.Println("  rex version                                                  Show client release version")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --token, -t     Authentication bearer token")
	fmt.Println("  --tls           Enable TLS 1.3 encryption (Default: true)")
	fmt.Println("  --insecure, -k  Skip certificate verification (Self-signed)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  rex connect 5.202.5.134:7444 --token mysecrettoken")
	fmt.Println("  rex exec 5.202.5.134:7444 -t mytoken \"systemctl status x-ui\"")
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

func runConnect(args []string) {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	token := fs.String("token", "", "Authentication token")
	fs.StringVar(token, "t", "", "Authentication token (shorthand)")
	useTLS := fs.Bool("tls", true, "Use TLS 1.3 encryption")
	insecure := fs.Bool("insecure", true, "Skip TLS cert verification")
	fs.BoolVar(insecure, "k", true, "Skip TLS cert verification (shorthand)")

	// Separate flags (starts with -) from positional arguments (like @root 5.202.5.134)
	var flagArgs []string
	var posArgs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			posArgs = append(posArgs, a)
		}
	}

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

	var flagArgs []string
	var posArgs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			posArgs = append(posArgs, a)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		os.Exit(1)
	}

	if len(posArgs) < 2 {
		fmt.Fprintln(os.Stderr, "Error: Usage: rex exec <host:port> [flags] \"<command>\"")
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
			fmt.Fprintln(os.Stderr, "\nError: Authentication token required.")
			os.Exit(1)
		}
		*token = strings.TrimSpace(enteredToken)
		saveStoredToken(addr, *token)
	}

	client, err := Dial(addr, *token, *useTLS, *insecure)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Register stream 2 for Fast Exec
	ch := client.RegisterStream(2)
	defer client.UnregisterStream(2)

	// Send OpFastExec directly (Sub-millisecond latency, zero PTY overhead)
	if err := client.Send(OpFastExec, 2, []byte(remoteCmd)); err != nil {
		fmt.Fprintf(os.Stderr, "❌ FastExec failed: %v\n", err)
		os.Exit(1)
	}

	// Wait for execution result frame
	resFrame := <-ch
	if resFrame == nil {
		return
	}
	if resFrame.Opcode == OpError {
		fmt.Fprintf(os.Stderr, "❌ %s\n", string(resFrame.Payload))
		os.Exit(1)
	}
	os.Stdout.Write(resFrame.Payload)
}

func runInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	token := fs.String("token", "", "Authentication token")
	fs.StringVar(token, "t", "", "Authentication token (shorthand)")
	useTLS := fs.Bool("tls", true, "Use TLS 1.3 encryption")
	insecure := fs.Bool("insecure", true, "Skip TLS cert verification")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	posArgs := fs.Args()
	if len(posArgs) < 1 {
		fmt.Fprintln(os.Stderr, "Error: Missing <host:port> address")
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
