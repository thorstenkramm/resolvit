#!/usr/bin/env bash

set -e

if which apt-get; then
    sudo apt-get install -y -f python3-dnspython
fi

# Repeatedly verify cached negative TTLs with a deterministic clock before load testing.
go test -race -count=100 ./pkg/dnscache -run '^TestDNSCacheNegativeAnswer'

# Build the software with race condition detector enabled.
go build -race -o ./resolvit main.go
echo "✅ Build resolvit"
echo ""

# Create some local records
cat << EOF > ./records.txt
*.test.example.com CNAME test.example.com
test.example.com A 192.168.1.1
loop-a.test.example.com CNAME loop-b.test.example.com
loop-b.test.example.com CNAME loop-a.test.example.com
EOF

cat ./records.txt

LOG=/tmp/resolvit.log
test -e $LOG && rm -f $LOG
# Create config file
cat << EOF > /tmp/resolvit.conf
[server]
listen = "127.0.0.1:5300"

[upstream]
servers = ["9.9.9.9:53"]

[logging]
level = "info"
file = "$LOG"

[records]
resolve_from = "./records.txt"
EOF
# Start in the background
RESOLVIT_CONFIG=/tmp/resolvit.conf ./resolvit &
RESOLVIT_PID=$!
# Never leave the server behind when a test run fails
trap 'kill "$RESOLVIT_PID" 2>/dev/null || true' EXIT
echo "✅ Started resolvit"
echo ""

# A single request first: without loop detection each request for a looping name keeps a CPU core
# busy and allocates memory until the server is killed.
echo "🚛 Checking CNAME loop detection ..."
./dns-stress.py --server 127.0.0.1 --port 5300 --query loop-a.test.example.com \
  --expect-rcode SERVFAIL \
  --num-requests 1 --concurrency 1
echo "✅ CNAME loop detected"
echo ""

echo "🚛 Starting CNAME loop stress test ..."
./dns-stress.py --server 127.0.0.1 --port 5300 --query loop-a.test.example.com \
  --expect-rcode SERVFAIL \
  --num-requests 5000 --concurrency 100
echo "✅ CNAME loop stress test completed"
echo ""

echo "🚛 Starting AAAA stress test ..."
./dns-stress.py --server 127.0.0.1 --port 5300 --query test.example.com \
  --qtype AAAA --expect-empty \
  --num-requests 5000 --concurrency 100
echo "✅ AAAA stress test completed"
echo ""

# Run stress test
echo "🚛 Starting first stress test ..."
./dns-stress.py --server 127.0.0.1 --port 5300 --query test.example.com \
  --expect-content 192.168.1.1 \
  --num-requests 50000 --concurrency 500
echo "✅ First stress test completed"
echo ""

echo "🚛 Starting second stress test ..."
./dns-stress.py --server 127.0.0.1 --port 5300 --query "%RAND%.test.example.com" \
  --expect-content 192.168.1.1 \
  --num-requests 50000 --concurrency 500
echo "✅ Second stress test completed"
echo ""

echo "🚛 Shutting down resolvit"
pkill resolvit
echo ""

echo "🚛 Examining logs"
cat $LOG
