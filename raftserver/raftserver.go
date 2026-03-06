package main

import (
	//	"encoding/json"
	"bufio"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	//"net"
)

// server states
// https://go.dev/ref/spec#Iota
type ServerState int

const (
	Candidate ServerState = iota
	Follower
	Leader
	Failed
)

/* --------------- Find file and consume data --------------------*/
func fileParse(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	var hosts []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		hosts = append(hosts, scanner.Text())
	}

	return hosts, scanner.Err()
}

func validateServer(identity string, hosts []string) bool {
	/*
		host, port, err := net.SplitHostPort(identity)
		if err!=nil{
			return err
		}
	*/

	return slices.Contains(hosts, identity)

	// return nil
}

/* --------------- Logging --------------------*/

// https://www.golinuxcloud.com/golang-log-to-file/
func logging(logcommand string) {
	fileName := "server-host-port.log"
	logFile, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		log.Panic(err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.Println(logcommand)
}

type LogEntry struct {
	Index       int
	Term        int
	CommandName string
}

func formatLogging(logdata LogEntry) {
	output := fmt.Sprintf("%d,%d,%s", logdata.Term, logdata.Index, logdata.CommandName)
	logging(output)
}

type AppendEntriesRequest struct {
	Term         int
	PrevLogIndex int
	PrevLogTerm  int
	LeaderCommit int
	LeaderId     string
	LogEntries   []LogEntry
}

type AppendEntriesResponse struct {
	Term    int
	Success bool
}

type RequestVoteRequest struct {
	Term         int
	LastLogIndex int
	LastLogTerm  int
	CommandName  string
}

type RequestVoteResponse struct {
	Term        int
	VoteGranted bool
}

func main() {
	/*
		if len(os.Args) < 3 {
			fmt.Println("check num args")
			return
		}
	*/

	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "expect two arguments -> go run raftserver.go server-host:server-port filename")
		os.Exit(1)
	}

	identity := os.Args[1]
	path := os.Args[2]

	fmt.Println(identity, path)

	hostlist, hostreadErr := fileParse(path)
	if hostreadErr != nil {
		fmt.Println("file error:", hostreadErr)
		return
	}
	// sancheck := validateServer(identity, hostlist)
	if !validateServer(identity, hostlist) {
		fmt.Println("Server identity not in hostlist")
		return
	}

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

		case "log":
			os.Exit(1)
		case "print":
			os.Exit(1)
		case "resume":
			os.Exit(1)
		case "suspend":
			os.Exit(1)

		default:
			fmt.Printf("Command not understood: %s\n", cmd)
		}
	}

}
