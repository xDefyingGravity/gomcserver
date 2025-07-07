# gomcserver

A lightweight Go library for creating and managing Minecraft servers programmatically.

## Features

- Create and configure Minecraft servers easily
- Manage world loading and backups
- Control server lifecycle (start, stop, restart)
- Simple API designed for Go developers

## Installation

```bash
go get github.com/xDefyingGravity/gomcserver
```

## Usage
```go
package main

import (
    "fmt"
    "time"

    "github.com/xDefyingGravity/gomcserver"
)

func main() {
	// Create server instance
	srv := server.NewServer("my_mc_server", "1.21.5")

	// Accept EULA (mandatory)
	srv.AcceptEULA()

	// Set server properties
	srv.SetProperty("max-players", "10")
	srv.SetProperty("difficulty", "normal")

	// Get and print a property
	if val, ok := srv.GetProperty("max-players"); ok {
		fmt.Println("max-players =", val)
	}

	// Register event listeners
	srv.SetEventListener("stdout", func(line string) {
		fmt.Print("[stdout] ", line)
	})
	srv.SetEventListener("stderr", func(line string) {
		fmt.Print("[stderr] ", line)
	})
	srv.SetEventListener("playerJoin", func(name string, count int) {
		fmt.Printf("player joined: %s, players online: %d\n", name, count)
	})
	srv.SetEventListener("playerLeave", func(name string, count int) {
		fmt.Printf("player left: %s, players online: %d\n", name, count)
	})

	// Check running state and PID before start
	fmt.Println("is running?", srv.IsRunning())
	fmt.Println("PID before start:", srv.GetPID())

	// Start the server with default options
	err := srv.Start(nil)
	if err != nil {
		panic(fmt.Sprintf("failed to start server: %v", err))
	}

	// Check running state and PID after start
	fmt.Println("is running?", srv.IsRunning())
	fmt.Println("PID after start:", srv.GetPID())

	// Send commands
	_ = srv.SendCommand("say Hello from Go!")
	_ = srv.SetDifficulty("hard")
	_ = srv.SetWeather("clear")
	_ = srv.SetTime("noon")

	// Get all properties
	props := srv.GetProperties()
	fmt.Println("all properties:\n", props.String())

	// Retrieve runtime stats after a delay (to gather data)
	time.Sleep(2 * time.Second)
	stats, err := srv.GetStats()
	if err != nil {
		fmt.Println("failed to get stats:", err)
	} else {
		fmt.Printf("stats: cpu=%.2f%% mem=%.2fMB threads=%d uptime=%s\n",
			stats.CPUPercent, stats.MemoryMB, stats.ThreadCount, stats.Uptime)
	}

	// Backup synchronously (nonBlocking = false)
	if err := srv.Backup(false); err != nil {
		fmt.Println("backup error:", err)
	} else {
		fmt.Println("backup success")
	}

	// Backup asynchronously (nonBlocking = true)
	err = srv.Backup(true)
	if err != nil {
		fmt.Println("async backup error:", err)
	} else {
		fmt.Println("async backup started")
	}

	// Stop the server
	if err := srv.Stop(); err != nil {
		fmt.Println("failed to stop server:", err)
	} else {
		fmt.Println("server stopped cleanly")
	}
}
```