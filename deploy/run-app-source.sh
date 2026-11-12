#!/usr/bin/env bash
set -euo pipefail

cd /source
export CGO_ENABLED=1
export CGO_CFLAGS=-Wno-deprecated-declarations
export GOPROXY=off
go build -buildvcs=false -ldflags="-X google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn" -o /home/appuser/WeKnora ./cmd/server
exec /home/appuser/WeKnora
