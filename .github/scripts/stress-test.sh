#!/usr/bin/env bash

set -e

if which apt-get; then
    sudo apt-get install -y -f python3-dnspython
fi

# Build the software with race condition detector enabled.
go build -race -o ./resolvit main.go
echo "✅ Build resolvit"
echo ""

# Create some local records
cat << EOF > ./records.txt
*.test.example.com CNAME test.example.com
test.example.com A 192.168.1.1
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
echo "✅ Started resolvit"
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
