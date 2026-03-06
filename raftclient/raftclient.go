package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	/*
		if len(os.Args) < 3 {
			fmt.Println("check num args")
			return
		}
	*/

	// loop and wait for user inputs (Reference: minichord.pdf)
	reader := bufio.NewReader(os.Stdin)

	for {
		cmd, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			break
		}
		cmd = strings.TrimSpace(cmd)

		// guard input
		tok := strings.Fields(cmd)
		if len(tok) == 0 {
			continue
		}

		switch tok[0] {

		case "exit":

		default:

		}
	}

}
