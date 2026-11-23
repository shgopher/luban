#!/usr/bin/env bash
# 编译本章实验二进制（输出到本目录，正文命令里的 ./s664-* 就是它们）
set -euo pipefail
export GOMODCACHE=/Users/shgopher/Desktop/family/luban/.cache/ch66/gomodcache
export GOCACHE=/Users/shgopher/Desktop/family/luban/.cache/gocache
export GOBIN=/Users/shgopher/Desktop/family/luban/.cache/ch66/bin
export GOFLAGS=-mod=mod GOWORK=off GOPROXY=off GOSUMDB=off
cd "$(dirname "$0")"
go build -o s664-turnserver ./turnserver
go build -o s664-expA ./expA
go build -o s664-expC ./expC
echo "built: s664-turnserver s664-expA s664-expC"
