package main

import (
	"fmt"
	"github.com/xDefyingGravity/gomcserver"
	"io"
	"time"
)

func main() {
	myServer := gomcserver.NewServer("my_server", "1.21.7")
	myServer.AcceptEULA()

	_, pw := io.Pipe()

	if err := myServer.SetEventListener("stdout", func(output string) {
		fmt.Print("[stdout] " + output)
	}); err != nil {
		fmt.Println("[error] failed to set stdout listener:", err)
		return
	}

	if err := myServer.SetEventListener("playerJoin", func(playerName string, newCount int) {
		fmt.Println("[player join] " + playerName)
	}); err != nil {
		fmt.Println("[error] failed to set playerJoin listener:", err)
		return
	}

	if err := myServer.SetEventListener("playerLeave", func(playerName string, newCount int) {
		fmt.Println("[player leave] " + playerName)
	}); err != nil {
		fmt.Println("[error] failed to set playerLeave listener:", err)
	}

	myServer.SetProperty("gamemode", "creative")

	if err := myServer.Start(&gomcserver.StartOptions{
		StdoutPipe: pw,
		StderrPipe: io.Discard,
	}); err != nil {
		fmt.Println("[error] failed to start server:", err)
		return
	}

	ticks := 0
	for {
		if !myServer.IsRunning() {
			fmt.Println("[log] server is not running, exiting...")
			break
		}

		ticks++
		time.Sleep(1 * time.Second)
	}

	_ = pw.Close()
}
