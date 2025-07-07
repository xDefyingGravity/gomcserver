package main

import (
	"fmt"
	"io"
	"time"
)

func main() {
	myServer := NewServer("my_server", "1.21.5")
	myServer.AcceptEULA()

	_, pw := io.Pipe()

	if err := myServer.SetEventListener("stdout", func(output string) {
		fmt.Print("[stdout] " + output)
	}); err != nil {
		fmt.Println("[error] failed to set stdout listener:", err)
		return
	}

	if err := myServer.SetEventListener("playerJoin", func(output string, newCount int) {
		fmt.Println("[player join] " + output)
	}); err != nil {
		fmt.Println("[error] failed to set playerJoin listener:", err)
		return
	}

	myServer.SetProperty("gamemode", "creative")

	if err := myServer.Start(&StartOptions{
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

		if ticks%20 == 0 {
			err := myServer.Backup(true)
			if err != nil {
				return
			}
		}

		ticks++
		time.Sleep(1 * time.Second)
	}

	_ = pw.Close()
}
