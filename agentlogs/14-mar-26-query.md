<copy paste of the server side code>
draw a decision tree in latex for me to track the state machine logic of my code

\documentclass[tikz,border=10pt]{standalone}
\usepackage{tikz}
\usetikzlibrary{arrows.meta,positioning,shapes.geometric}

\tikzset{
  block/.style={
    rectangle, rounded corners,
    draw, align=left,
    text width=5.2cm,
    minimum height=8mm
  },
  decision/.style={
    diamond, draw, align=center,
    aspect=2, text width=3.6cm,
    inner sep=1pt
  },
  line/.style={-{Latex[length=2mm]}, thick},
  lab/.style={midway, fill=white, inner sep=1pt}
}

\begin{document}
\begin{tikzpicture}[node distance=10mm and 14mm]

% Root
\node[block] (start) {Incoming event / periodic trigger};

% Top-level split
\node[decision, below=of start] (etype) {Event type?};

\node[block, below left=18mm and 28mm of etype] (recv) {UDP packet received\\$\to$ \texttt{Receive(data, sender)}};
\node[block, below=18mm of etype] (timeouttick) {25\,ms ticker\\$\to$ \texttt{Timeout()}};
\node[block, below right=18mm and 28mm of etype] (heartbeattick) {50\,ms ticker\\$\to$ \texttt{HeartbeatTick()}};

\draw[line] (start) -- (etype);
\draw[line] (etype) -- node[lab] {packet} (recv);
\draw[line] (etype) -- node[lab] {election check} (timeouttick);
\draw[line] (etype) -- node[lab] {leader heartbeat} (heartbeattick);

% ---------------- Receive path ----------------
\node[decision, below=of recv] (failedrecv) {state == Failed?};
\node[block, below left=12mm and 20mm of failedrecv] (dropfailed) {Return immediately};
\node[block, below right=12mm and 20mm of failedrecv] (unmarshal) {Unmarshal packet\\into one of:\\ClientCommand\\AppendEntriesReq\\AppendEntriesResp\\RequestVoteReq\\RequestVoteResp};

\draw[line] (recv) -- (failedrecv);
\draw[line] (failedrecv) -- node[lab] {yes} (dropfailed);
\draw[line] (failedrecv) -- node[lab] {no} (unmarshal);

\node[decision, below=of unmarshal] (msgkind) {Message type?};

\draw[line] (unmarshal) -- (msgkind);

% Message type branches
\node[block, below left=16mm and 36mm of msgkind] (clientmsg) {\texttt{handleClientCommand(data)}};
\node[block, below left=16mm and 12mm of msgkind] (aereqmsg) {\texttt{HandleAppendEntriesRequest(req)}\\then \texttt{Send(resp, sender)}};
\node[block, below=16mm of msgkind] (aerespmsg) {\texttt{HandleAppendEntriesResponse(resp, sender)}};
\node[block, below right=16mm and 12mm of msgkind] (rvreqmsg) {\texttt{HandleRequestVoteRequest(req)}\\then \texttt{Send(resp, sender)}};
\node[block, below right=16mm and 36mm of msgkind] (rvrespmsg) {\texttt{HandleRequestVoteResponse(resp, sender)}};

\draw[line] (msgkind) -- node[lab] {ClientCommand} (clientmsg);
\draw[line] (msgkind) -- node[lab] {AppendEntriesReq} (aereqmsg);
\draw[line] (msgkind) -- node[lab] {AppendEntriesResp} (aerespmsg);
\draw[line] (msgkind) -- node[lab] {RequestVoteReq} (rvreqmsg);
\draw[line] (msgkind) -- node[lab] {RequestVoteResp} (rvrespmsg);

% ---------------- handleClientCommand ----------------
\node[decision, below=of clientmsg] (clientstate) {Current state?};

\draw[line] (clientmsg) -- (clientstate);

\node[block, below left=16mm and 28mm of clientstate] (clientleader) {Leader:\\extract command\\append new local log entry\\for each peer:\\\texttt{AppendEntries(peer)}};
\node[block, below=16mm of clientstate] (clientcand) {Candidate:\\drop / ignore command};
\node[block, below right=16mm and 28mm of clientstate] (clientfollow) {Follower:\\if leader known, forward raw JSON\\to leader};

\draw[line] (clientstate) -- node[lab] {Leader} (clientleader);
\draw[line] (clientstate) -- node[lab] {Candidate} (clientcand);
\draw[line] (clientstate) -- node[lab] {Follower} (clientfollow);

% ---------------- Timeout ----------------
\node[decision, below=of timeouttick] (timeoutstate) {state == Failed\\or Leader?};
\node[block, below left=12mm and 18mm of timeoutstate] (timeoutret1) {Return};
\node[decision, below right=12mm and 18mm of timeoutstate] (expired) {time.Since(latestHeartbeat)\\< timeoutDuration?};

\draw[line] (timeouttick) -- (timeoutstate);
\draw[line] (timeoutstate) -- node[lab] {yes} (timeoutret1);
\draw[line] (timeoutstate) -- node[lab] {no} (expired);

\node[block, below left=12mm and 18mm of expired] (timeoutret2) {Return};
\node[block, below right=14mm and 20mm of expired] (startelection) {Election start:\\leaderID = ``''\\state = Candidate\\currentTerm++\\votedFor = self\\reset candidate vote maps\\self-vote\\reset election timeout\\send RequestVote to all peers};

\draw[line] (expired) -- node[lab] {yes} (timeoutret2);
\draw[line] (expired) -- node[lab] {no} (startelection);

% ---------------- HeartbeatTick ----------------
\node[decision, below=of heartbeattick] (hbstate) {state == Leader\\and not Failed?};
\node[block, below left=12mm and 20mm of hbstate] (hbret) {Return};
\node[block, below right=12mm and 20mm of hbstate] (hbsend) {For each peer:\\\texttt{AppendEntries(peer)}};

\draw[line] (heartbeattick) -- (hbstate);
\draw[line] (hbstate) -- node[lab] {no} (hbret);
\draw[line] (hbstate) -- node[lab] {yes} (hbsend);

% ---------------- HandleRequestVoteRequest ----------------
\node[decision, below=of rvreqmsg] (rvreqterm) {req.Term < currentTerm?};
\node[block, below left=12mm and 18mm of rvreqterm] (rvreqreject) {Reply false};
\node[block, below right=12mm and 18mm of rvreqterm] (rvrequpdate) {If req.Term > currentTerm:\\\texttt{UpdateTerm(req.Term)}\\Check vote conditions:\\(votedFor empty or same candidate)\\and candidate log at least as up-to-date\\If valid: grant vote, set votedFor,\\refresh latestHeartbeat};

\draw[line] (rvreqmsg) -- (rvreqterm);
\draw[line] (rvreqterm) -- node[lab] {yes} (rvreqreject);
\draw[line] (rvreqterm) -- node[lab] {no} (rvrequpdate);

% ---------------- HandleRequestVoteResponse ----------------
\node[decision, below=of rvrespmsg] (rvrespstate) {state == Candidate?};
\node[block, below left=12mm and 22mm of rvrespstate] (rvrespret1) {Return};
\node[decision, below right=12mm and 20mm of rvrespstate] (rvrespterm) {resp.Term relation\\to currentTerm};

\draw[line] (rvrespmsg) -- (rvrespstate);
\draw[line] (rvrespstate) -- node[lab] {no} (rvrespret1);
\draw[line] (rvrespstate) -- node[lab] {yes} (rvrespterm);

\node[block, below left=12mm and 24mm of rvrespterm] (rvrespret2) {If resp.Term < currentTerm:\\ignore};
\node[block, below=18mm of rvrespterm] (rvrespupdate) {If resp.Term > currentTerm:\\\texttt{UpdateTerm(resp.Term)}\\return};
\node[block, below right=12mm and 24mm of rvrespterm] (rvrespcount) {If same term and election still current:\\ignore duplicate responders\\record vote response\\if granted, count it\\if granted votes $\ge$ majority:\\\texttt{BecomeLeader()}};

\draw[line] (rvrespterm) -- node[lab] {<} (rvrespret2);
\draw[line] (rvrespterm) -- node[lab] {>} (rvrespupdate);
\draw[line] (rvrespterm) -- node[lab] {=} (rvrespcount);

% ---------------- HandleAppendEntriesRequest ----------------
\node[decision, below=of aereqmsg] (aereqterm) {req.Term < currentTerm?};
\node[block, below left=12mm and 18mm of aereqterm] (aereqreject1) {Reply false};
\node[block, below right=12mm and 20mm of aereqterm] (aereqstepdown) {If req.Term > currentTerm:\\\texttt{UpdateTerm(req.Term)}\\state = Follower\\leaderID = req.LeaderId\\latestHeartbeat = now};

\draw[line] (aereqmsg) -- (aereqterm);
\draw[line] (aereqterm) -- node[lab] {yes} (aereqreject1);
\draw[line] (aereqterm) -- node[lab] {no} (aereqstepdown);

\node[decision, below=of aereqstepdown] (aereqprevidx) {PrevLogIndex > log length?};
\node[block, below left=12mm and 18mm of aereqprevidx] (aereqreject2) {Reply false};
\node[decision, below right=12mm and 18mm of aereqprevidx] (aereqprevterm) {PrevLogIndex > 0\\and local term at PrevLogIndex\\!= PrevLogTerm?};

\draw[line] (aereqstepdown) -- (aereqprevidx);
\draw[line] (aereqprevidx) -- node[lab] {yes} (aereqreject2);
\draw[line] (aereqprevidx) -- node[lab] {no} (aereqprevterm);

\node[block, below left=12mm and 18mm of aereqprevterm] (aereqreject3) {Reply false};
\node[block, below right=12mm and 20mm of aereqprevterm] (aereqappend) {For each incoming entry:\\if conflict at same index, delete suffix\\append missing entries\\If LeaderCommit > commitIndex:\\commitIndex = min(LeaderCommit, len(log))\\apply committed entries\\Reply true};

\draw[line] (aereqprevterm) -- node[lab] {yes} (aereqreject3);
\draw[line] (aereqprevterm) -- node[lab] {no} (aereqappend);

% ---------------- HandleAppendEntriesResponse ----------------
\node[decision, below=of aerespmsg] (aerespstate) {state == Leader?};
\node[block, below left=12mm and 18mm of aerespstate] (aerespignore1) {Return};
\node[decision, below right=12mm and 18mm of aerespstate] (aerespterm) {resp.Term relation\\to currentTerm};

\draw[line] (aerespmsg) -- (aerespstate);
\draw[line] (aerespstate) -- node[lab] {no} (aerespignore1);
\draw[line] (aerespstate) -- node[lab] {yes} (aerespterm);

\node[block, below left=12mm and 22mm of aerespterm] (aerespignore2) {If resp.Term < currentTerm:\\ignore};
\node[block, below=18mm of aerespterm] (aerespupdate) {If resp.Term > currentTerm:\\\texttt{UpdateTerm(resp.Term)}\\return};
\node[decision, below right=12mm and 22mm of aerespterm] (aerespsuccess) {resp.Success?};

\draw[line] (aerespterm) -- node[lab] {<} (aerespignore2);
\draw[line] (aerespterm) -- node[lab] {>} (aerespupdate);
\draw[line] (aerespterm) -- node[lab] {=} (aerespsuccess);

\node[block, below left=12mm and 20mm of aerespsuccess] (aerespfail) {Failure:\\nextIndex[follower]--\\retry \texttt{AppendEntries(follower)}};
\node[block, below right=12mm and 20mm of aerespsuccess] (aerespok) {Success:\\update matchIndex / nextIndex\\advance commit index\\apply committed entries};

\draw[line] (aerespsuccess) -- node[lab] {no} (aerespfail);
\draw[line] (aerespsuccess) -- node[lab] {yes} (aerespok);

\end{tikzpicture}
\end{document}


space it out better, seperate them so that the tikz borders do not overlap

\documentclass[tikz,border=12pt]{standalone}
\usepackage{tikz}
\usetikzlibrary{arrows.meta,positioning,shapes.geometric}

\tikzset{
  block/.style={
    rectangle, rounded corners,
    draw, align=left,
    text width=6.0cm,
    minimum height=9mm,
    inner sep=4pt
  },
  decision/.style={
    diamond, draw, align=center,
    aspect=2.3,
    text width=4.2cm,
    inner sep=1.5pt
  },
  line/.style={-{Latex[length=2.2mm]}, thick},
  lab/.style={midway, fill=white, inner sep=1pt},
  every node/.style={font=\small}
}

\begin{document}
\begin{tikzpicture}[node distance=14mm and 24mm]

% Root
\node[block] (start) {Incoming event / periodic trigger};

% Top-level split
\node[decision, below=of start] (etype) {Event type?};

\node[block, below left=24mm and 48mm of etype] (recv) {UDP packet received\\$\to$ \texttt{Receive(data, sender)}};
\node[block, below=24mm of etype] (timeouttick) {25\,ms ticker\\$\to$ \texttt{Timeout()}};
\node[block, below right=24mm and 48mm of etype] (heartbeattick) {50\,ms ticker\\$\to$ \texttt{HeartbeatTick()}};

\draw[line] (start) -- (etype);
\draw[line] (etype) -- node[lab] {packet} (recv);
\draw[line] (etype) -- node[lab] {election check} (timeouttick);
\draw[line] (etype) -- node[lab] {leader heartbeat} (heartbeattick);

% ---------------- Receive path ----------------
\node[decision, below=of recv] (failedrecv) {state == Failed?};
\node[block, below left=18mm and 30mm of failedrecv] (dropfailed) {Return immediately};
\node[block, below right=18mm and 30mm of failedrecv] (unmarshal) {Unmarshal packet into one of:\\ClientCommand\\AppendEntriesReq\\AppendEntriesResp\\RequestVoteReq\\RequestVoteResp};

\draw[line] (recv) -- (failedrecv);
\draw[line] (failedrecv) -- node[lab] {yes} (dropfailed);
\draw[line] (failedrecv) -- node[lab] {no} (unmarshal);

\node[decision, below=of unmarshal] (msgkind) {Message type?};
\draw[line] (unmarshal) -- (msgkind);

\node[block, below left=22mm and 52mm of msgkind] (clientmsg) {\texttt{handleClientCommand(data)}};
\node[block, below left=22mm and 18mm of msgkind] (aereqmsg) {\texttt{HandleAppendEntriesRequest(req)}\\then \texttt{Send(resp, sender)}};
\node[block, below=22mm of msgkind] (aerespmsg) {\texttt{HandleAppendEntriesResponse(resp, sender)}};
\node[block, below right=22mm and 18mm of msgkind] (rvreqmsg) {\texttt{HandleRequestVoteRequest(req)}\\then \texttt{Send(resp, sender)}};
\node[block, below right=22mm and 52mm of msgkind] (rvrespmsg) {\texttt{HandleRequestVoteResponse(resp, sender)}};

\draw[line] (msgkind) -- node[lab] {ClientCommand} (clientmsg);
\draw[line] (msgkind) -- node[lab] {AppendEntriesReq} (aereqmsg);
\draw[line] (msgkind) -- node[lab] {AppendEntriesResp} (aerespmsg);
\draw[line] (msgkind) -- node[lab] {RequestVoteReq} (rvreqmsg);
\draw[line] (msgkind) -- node[lab] {RequestVoteResp} (rvrespmsg);

% ---------------- handleClientCommand ----------------
\node[decision, below=of clientmsg] (clientstate) {Current state?};
\draw[line] (clientmsg) -- (clientstate);

\node[block, below left=18mm and 36mm of clientstate] (clientleader) {Leader:\\extract command\\append new local log entry\\for each peer:\\\texttt{AppendEntries(peer)}};
\node[block, below=18mm of clientstate] (clientcand) {Candidate:\\drop / ignore command};
\node[block, below right=18mm and 36mm of clientstate] (clientfollow) {Follower:\\if leader known, forward raw JSON to leader};

\draw[line] (clientstate) -- node[lab] {Leader} (clientleader);
\draw[line] (clientstate) -- node[lab] {Candidate} (clientcand);
\draw[line] (clientstate) -- node[lab] {Follower} (clientfollow);

% ---------------- Timeout ----------------
\node[decision, below=of timeouttick] (timeoutstate) {state == Failed\\or Leader?};
\node[block, below left=18mm and 28mm of timeoutstate] (timeoutret1) {Return};
\node[decision, below right=18mm and 28mm of timeoutstate] (expired) {time.Since(latestHeartbeat)\\< timeoutDuration?};

\draw[line] (timeouttick) -- (timeoutstate);
\draw[line] (timeoutstate) -- node[lab] {yes} (timeoutret1);
\draw[line] (timeoutstate) -- node[lab] {no} (expired);

\node[block, below left=18mm and 28mm of expired] (timeoutret2) {Return};
\node[block, below right=20mm and 30mm of expired] (startelection) {Election start:\\leaderID = ``''\\state = Candidate\\currentTerm++\\votedFor = self\\reset candidate vote maps\\self-vote\\reset election timeout\\send RequestVote to all peers};

\draw[line] (expired) -- node[lab] {yes} (timeoutret2);
\draw[line] (expired) -- node[lab] {no} (startelection);

% ---------------- HeartbeatTick ----------------
\node[decision, below=of heartbeattick] (hbstate) {state == Leader\\and not Failed?};
\node[block, below left=18mm and 28mm of hbstate] (hbret) {Return};
\node[block, below right=18mm and 28mm of hbstate] (hbsend) {For each peer:\\\texttt{AppendEntries(peer)}};

\draw[line] (heartbeattick) -- (hbstate);
\draw[line] (hbstate) -- node[lab] {no} (hbret);
\draw[line] (hbstate) -- node[lab] {yes} (hbsend);

% ---------------- HandleRequestVoteRequest ----------------
\node[decision, below=of rvreqmsg] (rvreqterm) {req.Term < currentTerm?};
\node[block, below left=18mm and 28mm of rvreqterm] (rvreqreject) {Reply false};
\node[block, below right=18mm and 28mm of rvreqterm] (rvrequpdate) {If req.Term > currentTerm:\\\texttt{UpdateTerm(req.Term)}\\Check vote conditions:\\(votedFor empty or same candidate)\\and candidate log at least as up-to-date\\If valid: grant vote, set votedFor,\\refresh latestHeartbeat};

\draw[line] (rvreqmsg) -- (rvreqterm);
\draw[line] (rvreqterm) -- node[lab] {yes} (rvreqreject);
\draw[line] (rvreqterm) -- node[lab] {no} (rvrequpdate);

% ---------------- HandleRequestVoteResponse ----------------
\node[decision, below=of rvrespmsg] (rvrespstate) {state == Candidate?};
\node[block, below left=18mm and 30mm of rvrespstate] (rvrespret1) {Return};
\node[decision, below right=18mm and 30mm of rvrespstate] (rvrespterm) {resp.Term relation\\to currentTerm};

\draw[line] (rvrespmsg) -- (rvrespstate);
\draw[line] (rvrespstate) -- node[lab] {no} (rvrespret1);
\draw[line] (rvrespstate) -- node[lab] {yes} (rvrespterm);

\node[block, below left=18mm and 34mm of rvrespterm] (rvrespret2) {If resp.Term < currentTerm:\\ignore};
\node[block, below=22mm of rvrespterm] (rvrespupdate) {If resp.Term > currentTerm:\\\texttt{UpdateTerm(resp.Term)}\\return};
\node[block, below right=18mm and 34mm of rvrespterm] (rvrespcount) {If same term and election still current:\\ignore duplicate responders\\record vote response\\if granted, count it\\if granted votes $\ge$ majority:\\\texttt{BecomeLeader()}};

\draw[line] (rvrespterm) -- node[lab] {<} (rvrespret2);
\draw[line] (rvrespterm) -- node[lab] {>} (rvrespupdate);
\draw[line] (rvrespterm) -- node[lab] {=} (rvrespcount);

% ---------------- HandleAppendEntriesRequest ----------------
\node[decision, below=of aereqmsg] (aereqterm) {req.Term < currentTerm?};
\node[block, below left=18mm and 28mm of aereqterm] (aereqreject1) {Reply false};
\node[block, below right=18mm and 28mm of aereqterm] (aereqstepdown) {If req.Term > currentTerm:\\\texttt{UpdateTerm(req.Term)}\\state = Follower\\leaderID = req.LeaderId\\latestHeartbeat = now};

\draw[line] (aereqmsg) -- (aereqterm);
\draw[line] (aereqterm) -- node[lab] {yes} (aereqreject1);
\draw[line] (aereqterm) -- node[lab] {no} (aereqstepdown);

\node[decision, below=of aereqstepdown] (aereqprevidx) {PrevLogIndex > log length?};
\node[block, below left=18mm and 28mm of aereqprevidx] (aereqreject2) {Reply false};
\node[decision, below right=18mm and 28mm of aereqprevidx] (aereqprevterm) {PrevLogIndex > 0\\and local term at PrevLogIndex\\!= PrevLogTerm?};

\draw[line] (aereqstepdown) -- (aereqprevidx);
\draw[line] (aereqprevidx) -- node[lab] {yes} (aereqreject2);
\draw[line] (aereqprevidx) -- node[lab] {no} (aereqprevterm);

\node[block, below left=18mm and 28mm of aereqprevterm] (aereqreject3) {Reply false};
\node[block, below right=18mm and 30mm of aereqprevterm] (aereqappend) {For each incoming entry:\\if conflict at same index, delete suffix\\append missing entries\\If LeaderCommit > commitIndex:\\commitIndex = min(LeaderCommit, len(log))\\apply committed entries\\Reply true};

\draw[line] (aereqprevterm) -- node[lab] {yes} (aereqreject3);
\draw[line] (aereqprevterm) -- node[lab] {no} (aereqappend);

% ---------------- HandleAppendEntriesResponse ----------------
\node[decision, below=of aerespmsg] (aerespstate) {state == Leader?};
\node[block, below left=18mm and 30mm of aerespstate] (aerespignore1) {Return};
\node[decision, below right=18mm and 30mm of aerespstate] (aerespterm) {resp.Term relation\\to currentTerm};

\draw[line] (aerespmsg) -- (aerespstate);
\draw[line] (aerespstate) -- node[lab] {no} (aerespignore1);
\draw[line] (aerespstate) -- node[lab] {yes} (aerespterm);

\node[block, below left=18mm and 34mm of aerespterm] (aerespignore2) {If resp.Term < currentTerm:\\ignore};
\node[block, below=22mm of aerespterm] (aerespupdate) {If resp.Term > currentTerm:\\\texttt{UpdateTerm(resp.Term)}\\return};
\node[decision, below right=18mm and 34mm of aerespterm] (aerespsuccess) {resp.Success?};

\draw[line] (aerespterm) -- node[lab] {<} (aerespignore2);
\draw[line] (aerespterm) -- node[lab] {>} (aerespupdate);
\draw[line] (aerespterm) -- node[lab] {=} (aerespsuccess);

\node[block, below left=18mm and 30mm of aerespsuccess] (aerespfail) {Failure:\\nextIndex[follower]--\\retry \texttt{AppendEntries(follower)}};
\node[block, below right=18mm and 30mm of aerespsuccess] (aerespok) {Success:\\update matchIndex / nextIndex\\advance commit index\\apply committed entries};

\draw[line] (aerespsuccess) -- node[lab] {no} (aerespfail);
\draw[line] (aerespsuccess) -- node[lab] {yes} (aerespok);

\end{tikzpicture}
\end{document}


-> ended up not using it, i don't really trust whether it actually broke down the code into state machine, i'll just draw it manually and compare agains the pdf
