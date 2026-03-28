### How to Run Program: 

To run the program, you must create each server listed in ` serverlist.names ` with the specified hostname and port number: 

` go run raftserver/raftserver.go ${SERVER_HOST}:${SERVER_PORT} serverlist.names` 

Then, create clients that can communicate to a particular server previously created: 

` go run raftclient/raftclient.go ${SERVER_HOST}:${SERVER_PORT} ` 

### File Description: 

The submission contains the following files and folders: 

- raftclient/raftclient.go: The go file containing all logic for client nodes, including client commands 

- raftserver/raftserver.go: The go file containing all logic for server nodes, including leader elections and message passing 

- /agentlogs: A folder containing AI queries to make understanding the scope and a rubber duck to sanity check code understanding 

- AUTHORS: A text file containing group members and respective emails. 

- README: A text file describing the program and justification for program design. 

### Protocol Description - Ongaro and Ousterhout (2014) 

As per Ongaro and Ousterhout's Raft protocol, we implement consensus by first electing a leader, then giving the leader complete responsibility for managing the replicated log. The leader accepts log entries from clients, replicates them on other servers, and tells servers when it is safe to apply log entries to their state machines. Any other server that recieves log entries from clients forwards them to a leader once a leader exists.

Our implementation of this protocol prioritises decoding messages into separate MessageTypes, before calling the appropriate functions to handle them. This way, we can handle specific message types and simply check against the server's state and variables to execute the correct behaviour for its state, being able to implement raft safety properties within its specific use case in one place.

We also reference Diego Ongaro's dissertation for function names and implementation.

#### States

Each server has three main server states: Follower, Candidate and Leader. For the purpose of this implementation, a server can also be in a Failed State, which cannot vote or append logs. We keep a few states for server operations, elections, or log replication:

- PersistentState: Handles election data for followers, storing the current election term, who it has voted for in this term, and the logs for verification
- VolatileState: Handles log replication for followers, storing the highest log entry for majority servers and the server's actual highest log entry
- VolatileStateCandidate: Handles election data for candidates, storing the voted term and servers who voted
- VolatileStateLeader: Handles log replication for leaders, storing its most updated log index and the most updated log index of followers

#### Leader election 

Each server has a electionTicker that is created on initialisation in 'main()' on line 1113, which calls the 'Timeout()' function on line 563 and checks if the random timeout duration for this server exceeds the time passed since the latest heartbeat. When this happens, the server will increments its currentTerm, become a Candidate, and vote for itself, by calling 'initialiseVCState()' on line 843 and storing itself as a granted vote. Finally, it sends a RequestVote RPC to all of its peers. This signifies the start of an election term.

All servers regardless of state will listen out for RequestVote RPCs via 'Receive()' on line 206, looking for 'MessageType RequestVoteRequestMessage' which calls for 'HandleRequestVoteRequest()' function on line 695. This function handles all the logic regarding how a server will vote, as explained by Ongaro and Ousterhout:

- 1: returns false if the candidate's term is less than the server's current term, ensuring a server which timed out and is now outdated cannot be a leader
- 2: returns no vote if already voted for another server / this server, ensuring only one vote per server
- 3: returns no vote if the candidate's logs are not at least up-to-date with server's logs

If the candidate passes these requirements, then the server will vote them, and send a 'RequestVoteResponse()' back. It will also reset its own latestHeartbeat, since it has recieved a response.

The candidate will listen out for these responses via 'Receive()' on line 206, looking for 'MessageType RequestVoteResponseMessage' which calls for 'HandleRequestVoteResponse()' function on line 731. If it receives a vote from a particular server, it will store this information in the VolatileStateCandidate created at the start of the leader election. Finally, if the number of votes granted exceeds the majority of the number of peers in the network, then it will call 'BecomeLeader()' on line 660 to win the elections.

The function 'Timeout()' will handles the case where the candidate does not recieve enough votes in the election term, since it will reset to a new VolatileStateCandidate when it increments its currentTerm, become a Candidate, and vote for itself. This also applies to the case of multiple candidates in an election term, since both will eventually reach their own timeouts.
  
#### Log replication 

When a server recieves a client command from 'MessageType ClientCommandMessage' in 'Receive()' on line 206, it will call 'handleClientCommand()' function on line 270. 

- Follower: The server will pass the raw message to the current leader server.
- Candidate: The server will append this command to its own command buffer, since there is no existing leader to forward the message to. Once it becomes a leader, it will handle all commands in its buffer on line , or forward all commands to the new leader on line .
- Leader: A leader will append the new commands to its logs, before calling 'AppendEntries()' on line 339 for all peers. Here, it checks the peer's log entries and sends as many entries as needed to update it to the leader's logs using a list of LogEntrys. Finally, it will send the logs as a 'MessageType AppendEntriesRequestMessage' using 'Send()' on line 176.

All servers regardless of state will listen out for AppendEntries RPCs via 'Receive()' on line 206, looking for 'MessageType AppendEntriesRequestMessage' which calls for 'HandleAppendEntriesRequest()' function on line 388. This function handles all the logic regarding how a server adds entries to its logs, as explained by Ongaro and Ousterhout:

- 1: returns false if the candidate's term is less than the server's current term, ensuring a server which timed out and is now outdated cannot send new logs
- 2: returns false if log doesn’t contain an entry at prevLogIndex
whose term matches prevLogTerm
- 3: if an existing entry conflicts with a new one, having the same index number, delete that entry and all entires after, since we assume leader's logs are the truth
- 4: appends all entries not in the temporary logs in PersistentState after solving conflicts
- 5: fix commit value by setting commitIndex to the minimum between leader's commitIndex and last commit in logs

Once the entries have been updated, the server will send a 'MessageType AppendEntriesResponseMessage' back. A leader will listen out for this response via 'Recieve()', calling 'HandleAppendEntriesResponse()' on line 488. Here, we do a sanity check of updating the leader's term and state if it is outdated, only calling 'AdvanceCommitIndex()' on line 770 and 'ApplyCommitedEntries()' on line 476. This is where we permanently update the leader's logs, and by advancing the leader's commitIndex, and appending to the logs. In the next heartbeat, we also permanently update the follower's logs which can now succeed the leaderCommit check on line 465 to update its logs.


### Request & Response Tracking 

#### Voting Messages

Because of the timeout logic and the RequestVote RPC cycle, we can ensure that leader elections will proceed and eventually succeed if there are enough active servers. If a server does not recieve a heartbeat before its internal timeout, it will trigger a new election and request votes from all other servers, where active servers are always listening for this message and will respond with success or failure if they recieve it.

To ensure a server does not wait forever for votes, again the timeout logic ensures we can start a new election term and try requesting votes again until someone wins the majority votes and the election.

#### Logging Messages

An elected leader uses AppendEntries RPCs both to update follower logs and to send heartbeats to all followers, and uses the responses to track how up to date follower logs are in VolatileStateLeader. This is sent when a client command is registered, to update follower logs, or when a heartbeat is sent using an empty AppendEntries RPC to all followers. Follower will always send a response back to the leader, which can be a success if they can update their logs or a failure if one of the cases specified above is not fulfilled.

For followers who return negative responses, we handle the difference in logs by decrementing that follower's nextIndex, and immediately sending another AppendEntries RPC. This ensures that follower logs will eventually be made up to date and identical to the leader.

#### Client Messages

We ensure all messages are handled through the use of a commandBuffer, which stores commands sent if there are no current leaders, and message forwarding by followers to the current leader. This way, no requests are lost during leader elections.

### Heartbeat Mechanism 

We use timeouts for the heartbeat mechanism, by creating a heartbeatTicker on line 117 which calls 'HeartbeatTick()' on line 523. We ignore this ticker if we are not the leader, else we will send the empty AppendEntries RPC to all followers by calling 'AppendEntries()'. This timeout is a separate timeout to the electionTicker, to ensure leaders always try to send a heartbeat before the follower's election tickers can fail.

By reusing the AppendEntries RPC call, followers simply have to check when they recieve a 'MessageType AppendEntriesRequestMessage' and update their latestHeartbest to this time, thus being able to compare this value against their electionTicker values in 'Timeout()' and triggering a new election if required.

### Justifications 

#### Ensured Progress 

Specifically for leader elections, we ensure eventual resolution using the randomised timeouts. In the rare case where two servers have the same timeout value, trigger an election vote and don't manage to gain a majority, there is a low likelihood of a repeated case of candidates with the same timeout value consecutively, and so one candidate will very likely win the majority vote in the next election. 

#### Deadlock Free 

We manage access to a server's state machine mechanisms using a mutex lock on line 896, which eliminates the possibility of circular waiting as any function which requires access to these variables will first attempt to take the lock.