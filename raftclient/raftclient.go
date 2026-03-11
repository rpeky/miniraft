package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"unicode"
)

type ClientCommand struct {
	Command string `json:"Command"`
}

// Reference: https://www.geeksforgeeks.org/go-language/check-if-the-rune-is-a-unicode-punctuation-character-or-not-in-golang/
func MessageValidity(s string) bool {
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// Send message to given server
func SendMessage(addr string, data []byte) {
	udpAddress, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, udpAddress)
	if err != nil {
		return
	}
	defer conn.Close()

	_, err = conn.Write(data)
	if err != nil {
		fmt.Printf("Send error: %v\n", err)
	} else {
		fmt.Printf("Sent: %s\n", string(data))
	}
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "expect one argument -> go run raftclient/raftclient.go server-host:server-port")
		os.Exit(1)
	}

	serverAddress := os.Args[1]
	fmt.Printf("Started raft client targetting: %s", serverAddress)

	// loop and wait for user inputs (Reference: minichord.pdf)
	reader := bufio.NewReader(os.Stdin)

	for {
		cmd, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			break
		}
		cmd = strings.TrimSpace(cmd)

		// check for empty command
		tok := strings.Fields(cmd)
		if len(tok) == 0 {
			continue
		}

		// check for command validity -- no punctuation / spaces for all commands
		if !MessageValidity(cmd) {
			fmt.Println("Error: Only letters and digits are permitted (no spaces or punctuation).")
			continue
		}

		switch tok[0] {
		case "exit":
			return
		default: // if unrecognised, assume command to server
			payload := ClientCommand{Command: cmd}
			data, err := json.Marshal(payload)
			if err != nil {
				fmt.Printf("JSON Error: %v\n", err)
				continue
			}
			SendMessage(serverAddress, data)
		}

	}

}
