#!/usr/bin/bash

set -euo pipefail

rm -f server-host-port.log
go run raftserver/raftserver.go 127.0.0.1:2002 serverlist.names 
