package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
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
	ClientCommandMessage
)

type RaftMessage struct {
	Message any
}

func (message *RaftMessage) MarshalJson() (result []byte, err error) {
	result, err = json.Marshal(message.Message)
	return
}
func (message *RaftMessage) UnmarshalJSON(b []byte) (msg MessageType, err error) {
	var clientMap map[string]string
	err = json.Unmarshal(b, &clientMap)
	if err == nil {
		if value, ok := clientMap["Command"]; ok && value != "" {
			message.Message = value
			return ClientCommandMessage, nil
		}
	}

	aer := &AppendEntriesRequest{}
	err = json.Unmarshal(b, aer)
	if err == nil && aer.LeaderId != "" {
		msg = AppendEntriesRequestMessage
		message.Message = aer
		return msg, nil
	}

	if _, ok := err.(*json.UnmarshalTypeError); err != nil && !ok {
		return 0, err
	}

	var raw map[string]json.RawMessage
	err = json.Unmarshal(b, &raw)
	if err != nil {
		return 0, err
	}

	if _, ok := raw["Success"]; ok {
		aeres := &AppendEntriesResponse{}
		if err = json.Unmarshal(b, aeres); err != nil {
			return 0, err
		}
		message.Message = aeres
		return AppendEntriesResponseMessage, nil
	}

	rvr := &RequestVoteRequest{}
	if err = json.Unmarshal(b, rvr); err == nil && rvr.CandidateName != "" {
		message.Message = rvr
		return RequestVoteRequestMessage, nil
	}

	if _, ok := err.(*json.UnmarshalTypeError); err != nil && !ok {
		return 0, err
	}

	if _, ok := raw["VoteGranted"]; ok {
		rvres := &RequestVoteResponse{}
		if err = json.Unmarshal(b, rvres); err != nil {
			return 0, err
		}
		message.Message = rvres
		return RequestVoteResponseMessage, nil
	}

	return 0, fmt.Errorf("unknown message type")
	/*
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
	*/
}

func DropStaleResponse() {
	// TODO
}

// Send messages to other servers only
func (sm *ServerSM) Send(response any, target string) {
	if sm.conn == nil {
		return
	}

	rm := RaftMessage{Message: response}
	data, err := rm.MarshalJson()
	if err != nil {
		log.Printf("marshal json error :%v", err)
		return
	}

	if len(data) >= 1400 {
		log.Printf("message too large: %d bytes", len(data))
		return
	}

	udpAddress, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return
	}

	_, err = sm.conn.WriteToUDP(data, udpAddress)
	if err != nil {
		log.Printf("udp write/send error :%v", err)
		return
	}
}

// Recieve messages from client / other servers
func (sm *ServerSM) Receive(data []byte, senderAddress *net.UDPAddr) {
	sm.mu.Lock()
	if sm.state == Failed {
		sm.mu.Unlock()
		return
	}
	sm.mu.Unlock()

	var rm RaftMessage
	msgType, umjErr := rm.UnmarshalJSON(data)
	if umjErr != nil {
		log.Printf("iUnmarshallJson error: %v", umjErr)
		return
	}
	log.Printf("[%s] RECV from=%s msgType=%d raw=%s", sm.identity, senderAddress.String(), msgType, string(data))

	switch msgType {
	case ClientCommandMessage:
		sm.handleClientCommand(data)

	case AppendEntriesRequestMessage:
		req, ok := rm.Message.(*AppendEntriesRequest)
		if !ok || req == nil {
			return
		}
		response := sm.HandleAppendEntriesRequest(req)
		sm.Send(response, senderAddress.String())

	case AppendEntriesResponseMessage:
		resp, ok := rm.Message.(*AppendEntriesResponse)
		if !ok || resp == nil {
			return
		}
		sm.HandleAppendEntriesResponse(resp, senderAddress.String())

	case RequestVoteRequestMessage:
		req, ok := rm.Message.(*RequestVoteRequest)
		if !ok || req == nil {
			return
		}
		response := sm.HandleRequestVoteRequest(*req)
		sm.Send(response, senderAddress.String())

	case RequestVoteResponseMessage:
		resp, ok := rm.Message.(*RequestVoteResponse)
		if !ok || resp == nil {
			return
		}
		sm.HandleRequestVoteResponse(resp, senderAddress.String())

	default:
		break
	}
}

func extractCommandJson(data []byte) (string, error) {
	var cmd map[string]string
	if err := json.Unmarshal(data, &cmd); err != nil {
		return "", err
	}
	return cmd["Command"], nil
}

// gonna refactor this bit from Receive
func (sm *ServerSM) handleClientCommand(data []byte) {
	sm.mu.Lock()

	switch sm.state {
	// if leader, append to logs and alert all clients about this log
	case Leader:
		cmd, _ := extractCommandJson(data)
		newLogEntry := LogEntry{ // create log entry
			Index:       len(sm.pstate.log) + 1,
			Term:        sm.pstate.currentTerm,
			CommandName: cmd,
		}

		// append to own entry
		sm.pstate.log = append(sm.pstate.log, newLogEntry)
		peers := append([]string(nil), sm.peers...)
		sm.mu.Unlock()

		for _, peer := range peers {
			go sm.AppendEntries(peer)

		}

	case Candidate: // either buffer these commands and process them after the election concludes, or drop them
		sm.mu.Unlock()
		DropMessage()

	case Follower:
		leaderID := sm.leaderID
		sm.mu.Unlock()
		if leaderID != "" {
			sm.Send(json.RawMessage(data), leaderID)
		}

	default:
		sm.mu.Unlock()
	}
}

func DuplicateMessage() {
	//TODO
}

func DropMessage() {
	//TODO
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

// used by leader to replicate log entries using leder data
// also meant to be a heartbeat
func (sm *ServerSM) AppendEntries(peer string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// leader logs is the truth, skip
	if sm.state != Leader {
		return
	}

	nextIdx := sm.lstate.nextIndex[peer]
	prevLogIndex := nextIdx - 1
	prevLogTerm := 0
	if prevLogIndex > 0 {
		prevLogTerm = sm.pstate.log[prevLogIndex-1].Term
	}

	var entries []LogEntry
	if nextIdx <= len(sm.pstate.log) {
		// update logs
		entries = []LogEntry{
			sm.pstate.log[nextIdx-1],
		}
	}

	req := AppendEntriesRequest{
		Term:         sm.pstate.currentTerm,
		LeaderId:     sm.identity,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		LogEntries:   entries,
		LeaderCommit: sm.vstate.commitIndex,
	}

	go sm.Send(req, peer)

}

func ClientRequest() {
	//TODO
	//put inside receive
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
	// return fail if smaller
	if RequestData.Term < sm.pstate.currentTerm {
		return resp
	}

	// update if larger
	if RequestData.Term > sm.pstate.currentTerm {
		sm.UpdateTerm(RequestData.Term)
	}

	resp.Term = sm.pstate.currentTerm

	// term is valid, update values
	// transition candidate to follower on leader contect, reset timer
	sm.state = Follower
	sm.leaderID = RequestData.LeaderId
	sm.latestHeartbeat = time.Now()
	resp.Term = sm.pstate.currentTerm

	// log does not have prevLogIndex 5.2
	if RequestData.PrevLogIndex > len(sm.pstate.log) {
		return resp
	}

	// check if prev log terms match
	if RequestData.PrevLogIndex > 0 && sm.pstate.log[RequestData.PrevLogIndex-1].Term != RequestData.PrevLogTerm {
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

	// AppendEntries RPC receiver step 5 -> fix commmit value
	if RequestData.LeaderCommit > sm.vstate.commitIndex {
		// set commit index to the min
		sm.vstate.commitIndex = min(RequestData.LeaderCommit, len(sm.pstate.log))
		sm.ApplyCommitedEntries()
	}

	// change to true to return
	resp.Success = true
	return resp
}

func (sm *ServerSM) ApplyCommitedEntries() {
	// it still feels weird that for is overloaded with while and extracting range looping lol
	for sm.vstate.lastApplied < sm.vstate.commitIndex {
		sm.vstate.lastApplied++
		entry := sm.pstate.log[sm.vstate.lastApplied-1]
		// check if there is an error
		if err := sm.formatLogging(entry); err != nil {
			log.Printf("ApplyCommitedEntries log write error: %v", err)
		}
	}
}

func (sm *ServerSM) HandleAppendEntriesResponse(resp *AppendEntriesResponse, follower string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != Leader {
		return
	}

	// stale message
	if sm.pstate.currentTerm > resp.Term {
		return
	}

	// not up to date, update logs
	if sm.pstate.currentTerm < resp.Term {
		sm.UpdateTerm(resp.Term)
		return
	}

	if resp.Success {
		// check if leader is behind
		if sm.lstate.nextIndex[follower] <= len(sm.pstate.log) {
			sm.lstate.matchIndex[follower] = sm.lstate.nextIndex[follower]
			sm.lstate.nextIndex[follower]++
		}
		sm.AdvanceCommitIndex()
		sm.ApplyCommitedEntries()
		return
	}

	sm.lstate.nextIndex[follower] = max(1, sm.lstate.nextIndex[follower]-1)
	go sm.AppendEntries(follower)
}

// leader needs to send heartbeat to retain leadership or an election will be called
func (sm *ServerSM) HeartbeatTick() {
	sm.mu.Lock()
	if sm.state != Leader || sm.state == Failed {
		sm.mu.Unlock()
		return
	}

	peers := append([]string(nil), sm.peers...)
	sm.mu.Unlock()

	for _, peer := range peers {
		go sm.AppendEntries(peer)
	}
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state == Failed || sm.state == Leader {
		return
	}

	// check heartbeat if action requried
	inbetween := time.Since(sm.latestHeartbeat)
	if inbetween < sm.timeoutDuration {
		return
	}

	// debug
	log.Printf("[%s] TIMEOUT currentTerm=%d state=%d -> start election", sm.identity, sm.pstate.currentTerm, sm.state)

	// expired
	// transition to candndate, increment term, reset vote tracking
	// vote for yourself, reset the VolatileCandidate states
	// update self voting
	sm.leaderID = ""
	sm.state = Candidate
	sm.pstate.currentTerm++
	sm.pstate.votedFor = sm.identity

	sm.cstate.initialiseVCState(sm.pstate.currentTerm)
	sm.cstate.votesResponded[sm.identity] = true
	sm.cstate.votesGranted[sm.identity] = true

	// debug
	log.Printf("[%s] BECAME CANDIDATE term=%d votedFor=%s lastIdx=%d lastTerm=%d",
		sm.identity, sm.pstate.currentTerm, sm.pstate.votedFor, sm.lastLogIndex(), sm.lastLogTerm())

	// use the helper for a random time
	sm.SetElectionTimeout()

	currentTerm := sm.pstate.currentTerm
	lastIdx := sm.lastLogIndex()
	lastTerm := sm.lastLogTerm()
	peers := append([]string(nil), sm.peers...)

	go func(term int, lidx int, lterm int, peers []string) {
		req := RequestVoteRequest{
			term,
			lidx,
			lterm,
			sm.identity,
		}

		for _, peer := range peers {
			// debug
			log.Printf("[%s] SEND RequestVote to=%s term=%d lastIdx=%d lastTerm=%d",
				sm.identity, peer, term, lidx, lterm)
			sm.Send(req, peer)
		}
	}(currentTerm, lastIdx, lastTerm, peers)
}

func (sm *ServerSM) UpdateTerm(newTerm int) {
	//TODO
	// RPC update currentTerm reset voted for convert to follower
	if newTerm <= sm.pstate.currentTerm {
		return
	}

	// reset to follower, nuke states for overwrite
	sm.pstate.currentTerm = newTerm
	sm.pstate.votedFor = ""
	sm.state = Follower
	sm.leaderID = ""

	sm.cstate = VolatileStateCandidate{}

	sm.lstate = VolatileStateLeader{
		make(map[string]int),
		make(map[string]int),
	}

}

// compare against
func checkLog(reqLogIdx int, reqLogTerm int, lastIdx int, lastTerm int) bool {
	// check terms, return if the req term is larger
	if reqLogTerm != lastTerm {
		return reqLogTerm > lastTerm
	}
	// check last idx
	return reqLogIdx >= lastIdx
}

func RequestVote() {
	//TODO
	// send to peers not already responded in current election

}

func (sm *ServerSM) BecomeLeader() {
	if sm.state != Candidate {
		return
	}

	sm.state = Leader
	sm.leaderID = sm.identity
	sm.lstate.initialiseVVState(sm.lastLogIndex(), sm.peers)
	sm.cstate = VolatileStateCandidate{}

	peers := append([]string(nil), sm.peers...)
	go func(peers []string) {
		for _, peer := range peers {
			// tell peers to append entries
			sm.AppendEntries(peer)
		}
	}(peers)
}

func (sm *ServerSM) HandleRequestVoteRequest(req RequestVoteRequest) RequestVoteResponse {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// debug
	log.Printf("[%s] RECV RequestVote from=%s reqTerm=%d reqLastIdx=%d reqLastTerm=%d myTerm=%d myLastIdx=%d myLastTerm=%d votedFor=%s",
		sm.identity, req.CandidateName, req.Term, req.LastLogIndex, req.LastLogTerm,
		sm.pstate.currentTerm, sm.lastLogIndex(), sm.lastLogTerm(), sm.pstate.votedFor)

	resp := RequestVoteResponse{
		sm.pstate.currentTerm,
		false,
	}

	// 5.1 return false if term < currentTerm
	if req.Term < sm.pstate.currentTerm {
		return resp
	}

	// update logs if the request if further ahead
	if req.Term > sm.pstate.currentTerm {
		sm.UpdateTerm(req.Term)
	}

	resp.Term = sm.pstate.currentTerm

	// vote for the candidate if the request is valid
	if (sm.pstate.votedFor == "" || sm.pstate.votedFor == req.CandidateName) && checkLog(req.LastLogIndex, req.LastLogTerm, sm.lastLogIndex(), sm.lastLogTerm()) {
		sm.pstate.votedFor = req.CandidateName
		sm.latestHeartbeat = time.Now()
		resp.VoteGranted = true
	}

	return resp
}

func (sm *ServerSM) HandleRequestVoteResponse(resp *RequestVoteResponse, voter string) {
	// check votes responded and votes granded
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != Candidate {
		return
	}

	if resp.Term < sm.pstate.currentTerm {
		return
	}

	if resp.Term > sm.pstate.currentTerm {
		sm.UpdateTerm(resp.Term)
		return
	}

	// stale message
	if sm.cstate.electerm != sm.pstate.currentTerm {
		return
	}

	// already voted
	if sm.cstate.votesResponded[voter] {
		return
	}

	sm.cstate.votesResponded[voter] = true
	if resp.VoteGranted {
		sm.cstate.votesGranted[voter] = true
	}

	if len(sm.cstate.votesGranted) >= majority(len(sm.peers)+1) {
		sm.BecomeLeader()
	}

}

func (sm *ServerSM) AdvanceCommitIndex() {
	// only Leader can modify the value
	if sm.state != Leader {
		return
	}

	lastIdx := sm.lastLogIndex()
	clustersize := len(sm.peers) + 1

	for n := lastIdx; n > sm.vstate.commitIndex; n-- {
		if sm.pstate.log[n-1].Term != sm.pstate.currentTerm {
			continue
		}

		// exclude self
		track := 1
		for _, peer := range sm.peers {
			if sm.lstate.matchIndex[peer] >= n {
				track++
			}
		}

		if track >= majority(clustersize) {
			sm.vstate.commitIndex = n
			return
		}
	}

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

type VolatileStateCandidate struct {
	electerm       int
	votesResponded map[string]bool
	votesGranted   map[string]bool
	/*
		voterLastLogIndex map[string]int
		voterLastLogTerm  map[string]int
	*/
}

type VolatileStateLeader struct {
	nextIndex  map[string]int
	matchIndex map[string]int
}

func (p *PersistentState) initialisePState() {
	p.currentTerm = 0
	p.votedFor = ""
}

func (vc *VolatileStateCandidate) initialiseVCState(term int) {
	vc.electerm = term
	vc.votesResponded = make(map[string]bool)
	vc.votesGranted = make(map[string]bool)

}

func (v *VolatileState) initialiseVState() {
	v.commitIndex = 0
	v.lastApplied = 0
}

func (vl *VolatileStateLeader) initialiseVVState(lastLog int, peers []string) {
	// poll everyone for match index
	vl.matchIndex = make(map[string]int)
	// set the new valyue for
	vl.nextIndex = make(map[string]int)

	for _, peer := range peers {
		vl.nextIndex[peer] = lastLog + 1
		vl.matchIndex[peer] = 0
	}
}

// consolidate the states/funcs of the state machine
type ServerSM struct {
	// server role
	state ServerState

	// store UDP listening socket
	conn *net.UDPConn

	// state management stuff
	identity string
	leaderID string
	peers    []string

	// states for server ops, candidate, leader tracking
	pstate PersistentState
	vstate VolatileState
	cstate VolatileStateCandidate
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

func majority(n int) int {
	return n/2 + 1
}

func (sm *ServerSM) lastLogIndex() int {
	return len(sm.pstate.log)
}

func (sm *ServerSM) lastLogTerm() int {
	if len(sm.pstate.log) == 0 {
		return 0
	}
	return sm.pstate.log[len(sm.pstate.log)-1].Term
}

func (sm *ServerSM) printLog() {
	/*
		fmt.Println("plaeholder log test")
		sample := LogEntry{
			Index:       0,
			Term:        1,
			CommandName: "test",
		}
		sm.formatLogging(sample)
	*/
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, entry := range sm.pstate.log {
		fmt.Printf("%d,%d,%s\n", entry.Term, entry.Index, entry.CommandName)
	}
}

func (sm *ServerSM) printStates() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

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
	fmt.Printf("nextIndex %v\n", sm.lstate.nextIndex)
	fmt.Printf("matchIndex %v\n", sm.lstate.matchIndex)
}

/*--------------------State Transitions-------------------------*/
func (sm *ServerSM) Restart() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.state = Follower
	sm.leaderID = ""
	sm.timeoutDuration = RandomTimeoutValue()
	sm.latestHeartbeat = time.Now()
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

		// store the UDP socket
		server.conn = listener

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

	// election timer main loop
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			server.Timeout()
		}
	}()

	// leader heartbeat
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			server.HeartbeatTick()
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
			server.Restart()
			fmt.Printf("server %s resuming, wait for next vote to do stuff\n", server.identity)

		case "suspend":
			server.mu.Lock()
			server.state = Failed
			server.mu.Unlock()

			fmt.Printf("suspended server %s\n", server.identity)

		case "q":
			server.printStates()
			return

		default:
			fmt.Printf("Command not understood: %s\n", cmd)
		}
	}
}
