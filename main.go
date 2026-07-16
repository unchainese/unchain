package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	// Parse subcommands
	args := os.Args[1:]
	if len(args) == 0 {
		args = append(args, "run") // default to "run" if no subcommand is provided
	}

	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case "run":
		runServer()
	case "install":
		installService()
	case "client":
		runClient()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Printf("Unknown subcommand: %s\n\n", subcommand)
		printHelp()
		os.Exit(1)
	}
}

func runServer() {
	c := Cfg() //using default config.toml file
	fd := SetupLogger(c)
	defer fd.Close()

	// Channel to listen for OS signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	app := NewApp(c, stop)
	app.PushNode()                 //register node info to the manager server
	app.PrintVLESSConnectionURLS() //for standalone node
	go app.Run()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	app.Shutdown(ctx)
}

func installService() {
	// Check if systemctl is available
	cmd := exec.Command("systemctl", "--version")
	if err := cmd.Run(); err != nil {
		fmt.Println("systemctl is not available on this system")
		return
	}

	// Get the current user
	currentUser, err := user.Current()
	if err != nil {
		fmt.Printf("Failed to get current user: %v\n", err)
		return
	}
	username := currentUser.Username

	// Get the executable path
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to get executable path: %v\n", err)
		return
	}

	dir := filepath.Dir(exe)

	// Service file content
	content := fmt.Sprintf(`[Unit]
Description=Vless over Websocket Proxy
After=network.target

[Service]
ExecStart=%s
Restart=always
User=%s
WorkingDirectory=%s

[Install]
WantedBy=multi-user.target
`, exe, username, dir)

	// Write the service file
	servicePath := "/etc/systemd/system/unchain.service"
	err = os.WriteFile(servicePath, []byte(content), 0644)
	if err != nil {
		fmt.Printf("Failed to write service file to %s: %v\n", servicePath, err)
		return
	}

	fmt.Printf("Service file created at %s\n", servicePath)

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		fmt.Printf("Failed to reload systemd: %v\n", err)
		return
	}

	// Enable the service
	if err := exec.Command("systemctl", "enable", "unchain").Run(); err != nil {
		fmt.Printf("Failed to enable service: %v\n", err)
		return
	}

	// Start the service
	if err := exec.Command("systemctl", "start", "unchain").Run(); err != nil {
		fmt.Printf("Failed to start service: %v\n", err)
		return
	}

	fmt.Println("Service installed and started successfully")
}

func runClient() {
	fmt.Println("Starting SOCKS5 client...")
	StartSocks5Server()
}

func printHelp() {
	fmt.Println("Unchain - A VLESS over WebSocket proxy server")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  unchain [subcommand] [flags]")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  run       Run the server (default)")
	fmt.Println("  install   Install the service")
	fmt.Println("  client    Run as SOCKS5 server as VPN client")
	fmt.Println("  help      Show this help message")
	fmt.Println()
	fmt.Println("Flags:")
	flag.PrintDefaults()
}
