# resolvit

A DNS Server that allows you to resolve specific DNS records locally while forwarding all other
requests to upstream DNS Servers.

It's main use-case is to act as a local forwarder on your intranet with the ability to resolve
some records locally.

## Features

- Local resolution of A and CNAME records
- No need to create zones
- DNS forwarding to upstream servers
- DNS response caching
- Configurable via command-line flags
- Supports wildcards for A and CNAME records

Local records are read to memory on service start. Changes to the records file require
a service reload.

Records remain in the cache according to the TTL given by the upstream server.  
The cache is in-memory. It gets lost on service restart.

## Warnings

resolvit is not an RFC compliant DNS Server. Do not use on the public internet. 

The following features are not supported:
 - DNSsec
 - local resolving of AAAA, TXT, MX, SOA records
 - Zone transfers

## Usage

Start the DNS Server:

    resolvit --listen 127.0.0.1:5300 --upstream 8.8.8.8

Configuration Options:

```
--listen: Listen address for DNS Server (default "127.0.0.1:5300")
--upstream: Upstream DNS Server (can specify multiple, default "9.9.9.9:53")
--resolve-from: File containing DNS records to resolve locally
--log-level: Log level (debug, info, warn, error) (default "info")
--log-file: Log file path (stdout for console) (default "stdout")
```

## Local Records File Format

The local records file should contain one record per line in the format:

`name type content`

Example:

    my.example.com A 127.0.0.99
    cname.example.com CNAME my.example.com

Invalid lines are ignored. On loading the records resolvit will log a warning but continues starting. 

## Limitations

Only A and CNAME records are supported for local resolution
All other record types are forwarded to upstream DNS Servers

## Example Usage

Create a records file:

    echo "local.dev A 127.0.0.1" > records.txt
    echo "www.local.dev CNAME local.dev" >> records.txt

Start the server with local records:

    resolvit --listen 127.0.0.1:5300 --upstream 8.8.8.8:53 --resolve-from records.txt

Test DNS resolution:

    dig @127.0.0.1 -p 5300 local.dev
    dig @127.0.0.1 -p 5300 www.local.dev

## Run from systemd

Create a systemd service file `/etc/systemd/system/resolvit.service`:

```ini
[Unit]
Description=Resolvit DNS Server
After=network.target

[Service]
EnvironmentFile=/etc/default/resolvit
Environment="LOG_LEVEL=info"
Type=simple
ExecStart=/usr/local/bin/resolvit \
  --listen 127.0.0.1:5300 \
  --upstream 8.8.8.8:53 \
  --log-level $LOG_LEVEL \
  --resolve-from /var/lib/resolvit/records.txt
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
User=resolvit
Group=resolvit
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

Consider replacing values by variables read from the specified environment file.

Enable and start the service:

    systemctl enable resolvit
    systemctl start resolvit

## Compile from sources

For local testing:

    VERSION=$(date +%Y.%m%d.%H%M%S)
    go build -ldflags "-X resolvit/pkg/version.ResolvitVersion=$VERSION" -o resolvit

For a release build with version and optimization flags:

    go build -ldflags "-s -w -X resolvit/pkg/version.ResolvitVersion=<VERSION>" -o resolvit

## Testing

> [!TIP]
> Use `./docker-run-tests.sh` to run all tests and quality gates in a single run.

All parts are covered by go unit tests. Run them with:

    go test -race ./...

CI/CD runs [jscpd](https://github.com/kucherenko/jscpd) a Copy/paste detector for programming source code.
Before pushing, run it locally:

    npx jscpd --pattern "**/*.go" --ignore "**/*_test.go" --threshold 0 --exitCode 1

Additionally, a python based stress test command-line utility `./dns-stress.py` is included.
Python 3.7+ and `dnspython` is required. Use `./dns-stress.py --help`.  
Example usage:

    ./dns-stress.py --server 10.248.157.10 \
    --query "%RAND%.test.example.com" \
    --expect-content 10.111.1.154 \
    --num-requests 500000 \
    --concurrency 500