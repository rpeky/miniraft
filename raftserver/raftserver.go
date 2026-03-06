package main

import (
	//	"encoding/json"
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"
	//"net"
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
	return slices.Contains(hosts, identity)
}

/* --------------- Logging --------------------*/

// just in case server-host-port.log meant host and port were placeholder names LOL
// will be some x.x.x.x:yyyyy format | only have to swap the : with a -
// https://www.golinuxcloud.com/golang-log-to-file/
func constructLogFile(id string) (*os.File, error) {
	parsedName := strings.ReplaceAll(id, ":", "-")
	fileName := "server-" + parsedName + ".log"
	logFile, err := os.OpenFile(fileName, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	return logFile, nil
}

type LogEntry struct {
	Index       int
	Term        int
	CommandName string
}

func (sm *ServerSM) formatLogging(logdata LogEntry) error {
	_, writeErr := fmt.Fprintf(sm.logFile, "%d,%d,%s\n", logdata.Term, logdata.Index, logdata.CommandName)
	return writeErr
}

type AppendEntriesRequest struct {
	Term         int
	PrevLogIndex int
	PrevLogTerm  int
	LeaderCommit int
	LeaderID     string
	LogEntries   []LogEntry
}

type AppendEntriesResponse struct {
	Term    int
	Success bool
}

type RequestVoteRequest struct {
	Term          int
	LastLogIndex  int
	LastLogTerm   int
	CandidateName string
}

type RequestVoteResponse struct {
	Term        int
	VoteGranted bool
}

type ServerState int

const (
	Follower ServerState = iota
	Candidate
	Leader
	Failed
)

// https://raft.github.io/raft.pdf
type PersistentState struct {
	currentTerm int
	votedFor    string
	log         []LogEntry
}

type VolatileState struct {
	commitIndex int
	lastApplied int
}

type VolatileStateLeader struct {
	nextIndex  map[string]int
	matchIndex map[string]int
}

func (p *PersistentState) initialisePState() {
	p.currentTerm = 0
	p.votedFor = ""
}

func (v *VolatileState) initialiseVState() {
	v.commitIndex = 0
	v.lastApplied = 0
}

func (vl *VolatileStateLeader) initialiseVVState(lastLog int, peers []string) {
	vl.nextIndex = make(map[string]int)
	vl.matchIndex = make(map[string]int)

	for _, peer := range peers {
		vl.nextIndex[peer] = lastLog + 1
		vl.matchIndex[peer] = 0
	}
}

// consolidate the states/funcs of the state machine
type ServerSM struct {
	// server role
	state ServerState

	// state management stuff
	identity string
	leaderID string
	peers    []string

	pstate PersistentState
	vstate VolatileState

	// leader stuff
	nextIndex  map[string]int
	matchIndex map[string]int

	logFile *os.File
}

func generatePeers(identity string, hostlist []string) []string {
	peers := make([]string, 0, len(hostlist)-1)
	for _, h := range hostlist {
		if h != identity {
			peers = append(peers, h)
		}
	}
	return peers
}

func initialiseSM(identity string, hostlist []string) (*ServerSM, error) {
	// validate identity
	if !validateServer(identity, hostlist) {
		fmt.Println("Server identity not in hostlist")
		return nil, fmt.Errorf("%s is not in hostlist", identity)
	}

	// create log file
	logFile, lfErr := constructLogFile(identity)
	if lfErr != nil {
		fmt.Println("Failed to construct log file")
		return nil, fmt.Errorf("FATAL | Failed to construct Log File: %w", lfErr)
	}

	// generate peer list
	peerlist := generatePeers(identity, hostlist)

	s := &ServerSM{
		// always start as follower
		state: Follower,

		identity: identity,
		leaderID: "",
		peers:    peerlist,

		pstate: PersistentState{
			currentTerm: 0,
			votedFor:    "",
			log:         make([]LogEntry, 0),
		},
		vstate: VolatileState{
			commitIndex: 0,
			lastApplied: 0,
		},

		nextIndex:  make(map[string]int),
		matchIndex: make(map[string]int),

		logFile: logFile,
	}

	return s, nil
}

func (sm *ServerSM) printLog() {
	fmt.Println("plaeholder log test")
	sample := LogEntry{
		Index:       0,
		Term:        1,
		CommandName: "test",
	}
	sm.formatLogging(sample)
}

func (sm *ServerSM) printStates() {
	fmt.Printf("identity: %s\n", sm.identity)
	fmt.Printf("state: %d\n", sm.state)
	fmt.Println("\t0 - Follower")
	fmt.Println("\t1 - Candidate")
	fmt.Println("\t2 - Leader")
	fmt.Println("\t3 - Failed")
	fmt.Printf("leaderID: %s\n", sm.leaderID)
	fmt.Printf("peers: %v\n", sm.state)
	fmt.Printf("current term: %d\n", sm.pstate.currentTerm)
	fmt.Printf("voted for: %s\n", sm.pstate.votedFor)
	fmt.Printf("commit index: %d\n", sm.vstate.commitIndex)
	fmt.Printf("last applied: %d\n", sm.vstate.lastApplied)
	fmt.Printf("log size: %d\n", len(sm.pstate.log))
}

/*--------------------main-------------------------*/
func main() {

	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "expect two arguments -> go run raftserver.go server-host:server-port filename")
		os.Exit(1)
	}

	identity := os.Args[1]
	path := os.Args[2]

	fmt.Println(identity, path)

	hostlist, hostreadErr := fileParse(path)
	if hostreadErr != nil {
		fmt.Println("FATAL | file error:", hostreadErr)
		os.Exit(-1)
	}

	// initialise the server
	server, iniErr := initialiseSM(identity, hostlist)
	if iniErr != nil {
		fmt.Fprintln(os.Stderr, "server initialisation setup failed:", iniErr)
		os.Exit(-1)
	}

	// eventually close the log file
	defer server.logFile.Close()

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
			server.printLog()

		case "print":
			server.printStates()

		case "resume":
			// default behaviour is to be a follower, reset leader, wait to vote
			server.state = Follower
			server.leaderID = ""
			fmt.Printf("server %s resuming, wait for next vote to do stuff\n", server.identity)

		case "suspend":
			server.state = Failed
			fmt.Printf("suspended server %s\n", server.identity)

		case "q":
			server.printStates()
			return

		default:
			fmt.Printf("Command not understood: %s\n", cmd)
		}
	}

}
