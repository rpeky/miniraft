package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"time"

	"log"
	"net"
	"sync"
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

/* --------------- Message Handling --------------------*/
type MessageType int

const (
	AppendEntriesRequestMessage MessageType = iota
	AppendEntriesResponseMessage
	RequestVoteRequestMessage
	RequestVoteResponseMessage
)

type RaftMessage struct {
	Message any
}

func (message *RaftMessage) MarshalJson() (result []byte, err error) {
	result, err = json.Marshal(message.Message)
	return
}

func (message *RaftMessage) UnmarshalJSON(b []byte) (msg MessageType, err error) {
	aer := &AppendEntriesRequest{}
	err = json.Unmarshal(b, aer)
	if err == nil && aer.LeaderId != "" {
		msg = AppendEntriesRequestMessage
		message.Message = aer
		return
	}
	if _, ok := err.(*json.UnmarshalTypeError); err != nil && !ok {
		return
	}

	aeres := &AppendEntriesResponse{}
	if err = json.Unmarshal(b, aeres); err != nil {
		return
	}
	if aeres.Term > 0 {
		msg = AppendEntriesResponseMessage
		message.Message = aeres
		return
	}

	rvr := &RequestVoteRequest{}
	if err = json.Unmarshal(b, rvr); err != nil {
		return
	}
	if rvr.Term > 0 {
		msg = RequestVoteRequestMessage
		message.Message = rvr
		return
	}

	rvres := &RequestVoteResponse{}
	if err = json.Unmarshal(b, rvres); err != nil {
		return
	}
	if rvres.Term > 0 {
		msg = RequestVoteResponseMessage
		message.Message = rvres
	}
	return
}

func DropStaleResponse() {
}

// Send messages to other servers only
func (sm *ServerSM) Send(response any, target string) {
	rm := RaftMessage{Message: response}
	data, _ := rm.MarshalJson()

	udpAddress, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return
	}

	conn, err := net.DialUDP("udp", nil, udpAddress)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.Write(data)
}

// Recieve messages from client / other servers
func (sm *ServerSM) Receive(data []byte, senderAddress *net.UDPAddr) {
	var rm RaftMessage
	msgType, err := rm.UnmarshalJSON(data)

	if err != nil { // unrecognised message == client command
		switch sm.state {
		case Leader: // if leader, append to logs and alert all clients about this log
			sm.mu.Lock()
			newMessage := LogEntry{ // create log entry
				Index:       len(sm.pstate.log) + 1,
				Term:        sm.pstate.currentTerm,
				CommandName: string(data),
			}

			// append to own entry
			sm.pstate.log = append(sm.pstate.log, newMessage)
			sm.mu.Unlock()

			// TODO: respond to client

			// Get data from current sm
			sm.mu.Lock()
			logs := sm.pstate.log
			currentTerm := sm.pstate.currentTerm
			commitIndex := sm.vstate.commitIndex
			sm.mu.Unlock()

			for _, peer := range sm.peers {
				nextIndex := sm.lstate.nextIndex[peer]

				if len(logs) >= nextIndex {
					prevLogIndex := nextIndex - 1
					prevLogTerm := 0
					if prevLogIndex > 0 {
						prevLogTerm = logs[prevLogIndex-1].Term
					}

					// Slice updated entries
					entriesToSend := make([]LogEntry, len(logs[nextIndex-1:]))
					copy(entriesToSend, logs[nextIndex-1:])

					// Create Request
					request := AppendEntriesRequest{
						Term:         currentTerm,
						LeaderId:     sm.identity,
						PrevLogIndex: prevLogIndex,
						PrevLogTerm:  prevLogTerm,
						LogEntries:   entriesToSend,
						LeaderCommit: commitIndex,
					}

					sm.Send(request, peer)
				}
			}
		case Candidate: // either buffer these commands and process them after the election concludes, or drop them
			DropMessage()
		default: // if not leader, dont append anything and forward it to the leader (for both follower and failed)
			sm.mu.Lock()
			leaderID := sm.leaderID
			sm.mu.Unlock()

			if leaderID != "" {
				leaderAddress, err := net.ResolveUDPAddr("udp", sm.leaderID)
				if err != nil {
					log.Fatalf("Failed to resolve: %v", err)
				}

				conn, _ := net.DialUDP("udp", nil, leaderAddress)
				defer conn.Close()
				conn.Write(data)
			} else {
				fmt.Println("Command received but leader is unknown so we will drop the message.")
				DropMessage()
			}
		}
	}

	switch msgType {
	case AppendEntriesRequestMessage:
		req := rm.Message.(*AppendEntriesRequest)
		response := sm.HandleAppendEntriesRequest(req)
		sm.Send(response, senderAddress.String())
	case AppendEntriesResponseMessage:
		resp := rm.Message.(*AppendEntriesResponse)
		sm.HandleAppendEntriesResponse(resp)
	case RequestVoteRequestMessage:
		break
	case RequestVoteResponseMessage:
		break
	default:
		break
	}
}

func DuplicateMessage() {
}

func DropMessage() {
}

/* --------------- Raft operations --------------------*/
// https://web.stanford.edu/~ouster/cgi-bin/papers/OngaroPhD.pdf

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

func AppendEntries() {
}

func ClientRequest() {
}

func (sm *ServerSM) checkTerm(index int) int {
	if index == 0 {
		return 0
	}
	return sm.pstate.log[index-1].Term
}

func (sm *ServerSM) HandleAppendEntriesRequest(RequestData *AppendEntriesRequest) AppendEntriesResponse {
	// lock the rw
	sm.mu.Lock()
	defer sm.mu.Unlock()

	resp := AppendEntriesResponse{
		sm.pstate.currentTerm,
		false,
	}

	// mismatch term 5.1
	if RequestData.Term < sm.pstate.currentTerm {
		return resp
	}

	// log does not have prevLogIndex 5.2
	if RequestData.PrevLogIndex > len(sm.pstate.log) {
		return resp
	}

	if RequestData.PrevLogIndex > 0 && sm.checkTerm(RequestData.PrevLogIndex) != RequestData.PrevLogTerm {
		return resp
	}

	// check conflict with existing
	// position to start adding logs
	indexPos := RequestData.PrevLogIndex + 1

	for i, entry := range RequestData.LogEntries {
		target := i + indexPos
		sliceidx := target - 1

		// if entries already exist
		if target <= len(sm.pstate.log) {
			// delete this and all after if conflict 5.3
			// then add entries not in log in one go, no need to
			// iterate (AppendEntries RPC receiver step 4)
			if sm.pstate.log[sliceidx].Term != entry.Term {
				sm.pstate.log = sm.pstate.log[:sliceidx]
				sm.pstate.log = append(sm.pstate.log, RequestData.LogEntries[i:]...)
				break
			}

			// nothing else to do, index is caught up
			continue
		}

		// append anything else
		sm.pstate.log = append(sm.pstate.log, RequestData.LogEntries[i:]...)
		break
	}

	// AppendEntries RPC receiver step 5
	if RequestData.LeaderCommit > sm.vstate.commitIndex {
		// set commit index to the min
		sm.vstate.commitIndex = min(RequestData.LeaderCommit, len(sm.pstate.log))
	}

	// change to true to return
	resp.Success = true

	return resp
}

func (sm *ServerSM) HandleAppendEntriesResponse(ResponseData *AppendEntriesResponse) {
}

/* --------------- Voting mechanism --------------------*/

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

// somewhere between 150-300ms fixed interval
func RandomTimeoutValue() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

func (sm *ServerSM) SetElectionTimeout() {
	sm.timeoutDuration = RandomTimeoutValue()
	sm.latestHeartbeat = time.Now()
}

func (sm *ServerSM) Timeout() {
	if sm.state == Failed {
		return
	}

	if sm.state == Leader {
		return
	}

	// check heartbeat if action requried
	inbetween := time.Since(sm.latestHeartbeat)
	if inbetween < sm.timeoutDuration {
		return
	}

	// expired

}

func UpdateTerm() {
}

func RequestVote() {
}

func BecomeLeader() {
}

func HandleRequestVoteRequest() {
}

func HandleRequestVoteResponse() {
}

/* --------------- State Machine Mechanisms --------------------*/

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
	p.currentTerm = -1
	p.votedFor = ""
}

func (v *VolatileState) initialiseVState() {
	v.commitIndex = -1
	v.lastApplied = -1
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
	lstate VolatileStateLeader

	// voting mechanism
	timeoutDuration time.Duration
	latestHeartbeat time.Time

	logFile *os.File

	// lock shared stuff
	mu sync.Mutex
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

		lstate: VolatileStateLeader{
			nextIndex:  make(map[string]int),
			matchIndex: make(map[string]int),
		},

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
	fmt.Printf("peers: %v\n", sm.peers)
	fmt.Printf("current term: %d\n", sm.pstate.currentTerm)
	fmt.Printf("voted for: %s\n", sm.pstate.votedFor)
	fmt.Printf("commit index: %d\n", sm.vstate.commitIndex)
	fmt.Printf("last applied: %d\n", sm.vstate.lastApplied)
	fmt.Printf("log size: %d\n", len(sm.pstate.log))
}

/*--------------------State Transitions-------------------------*/

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

	// listen for connections in separate goroutine -- so we can still look for user inputs while this runs
	// Reference: miniraft.pdf
	go func() {
		addr, err := net.ResolveUDPAddr("udp", server.identity)
		if err != nil {
			log.Fatalf("Failed to resolve: %v", err)
		}

		listener, err := net.ListenUDP("udp", addr)
		if err != nil {
			log.Fatalf("Failed to listen: %v", err)
		}

		for {
			buf := make([]byte, 65536)

			n, remoteAddress, err := listener.ReadFromUDP(buf)
			if err != nil {
				fmt.Printf("UDP read error: %v\n", err)
				continue
			}

			// Use another goroutine in case there is a slow request, so we can continue listening for messages
			go server.Receive(buf[:n], remoteAddress)
		}
	}()

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
