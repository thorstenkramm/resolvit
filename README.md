# resolvit

A DNS Server that allows you to resolve specific DNS records locally while
forwarding all other requests to upstream DNS Servers.

Its main use-case is to act as a local forwarder on your intranet with the
ability to resolve some records locally.

## Features

- Local resolution of A and CNAME records
- No need to create zones
- DNS forwarding to upstream servers
- DNS response caching
- Configurable via `/etc/resolvit/resolvit.conf` (TOML syntax)
- Supports wildcards for A and CNAME records
- Optional DNS filtering with blocklists and allowlist overrides

Local records are read to memory on service start. Changes to the records file
require a service reload.

Records remain in the cache according to the TTL given by the upstream server.  
The cache is in-memory. It gets lost on service restart.

## Filtering

Resolvit can optionally block malware/scam/porn domains with curated blocklists
and allowlist overrides. See `docs/filtering.md` for configuration details and
the list catalog.

## Warnings

resolvit is not an RFC compliant DNS Server. Do not use on the public internet.

The following features are not supported:

- DNSsec
- local resolving of AAAA, TXT, MX, SOA records
- Zone transfers

## Usage

1. Copy `resolvit.conf.example` to `/etc/resolvit/resolvit.conf` and edit the
   values.
2. Start the DNS server:

    resolvit

To use a non-default config file, set `RESOLVIT_CONFIG`:

    RESOLVIT_CONFIG=/path/to/resolvit.conf resolvit

## Local Records File Format

The local records file should contain one record per line in the format:

`name type content`

Example:

    my.example.com A 127.0.0.99
    cname.example.com CNAME my.example.com

Invalid lines are ignored. On loading the records resolvit will log a warning
but continues starting.

## Limitations

Only A and CNAME records are supported for local resolution.
All other record types are forwarded to upstream DNS Servers.

## Example Usage

Create a records file:

    echo "local.dev A 127.0.0.1" > records.txt
    echo "www.local.dev CNAME local.dev" >> records.txt

Update `/etc/resolvit/resolvit.conf` with the records path:

    [records]
    resolve_from = "/path/to/records.txt"

Start the server:

    resolvit

Test DNS resolution:

    dig @127.0.0.1 -p 5300 local.dev
    dig @127.0.0.1 -p 5300 www.local.dev

## Run from systemd

Create a systemd service file
`/etc/systemd/system/resolvit.service`:

    [Unit]
    Description=Resolvit DNS Server
    After=network.target

    [Service]
    Type=simple
    Environment="RESOLVIT_CONFIG=/etc/resolvit/resolvit.conf"
    ExecStart=/usr/local/bin/resolvit
    ExecReload=/bin/kill -HUP $MAINPID
    Restart=on-failure
    User=resolvit
    Group=resolvit
    AmbientCapabilities=CAP_NET_BIND_SERVICE

    [Install]
    WantedBy=multi-user.target

If you store the config elsewhere, adjust `RESOLVIT_CONFIG` accordingly.

Enable and start the service:

    systemctl enable resolvit
    systemctl start resolvit

## Compile from sources

For local testing:

    VERSION=$(date +%Y.%m%d.%H%M%S)
    go build -ldflags \
        "-X resolvit/pkg/version.ResolvitVersion=$VERSION" \
        -o resolvit

For a release build with version and optimization flags:

    go build -ldflags \
        "-s -w -X resolvit/pkg/version.ResolvitVersion=<VERSION>" \
        -o resolvit

## Testing

> [!TIP]
> Use `./docker-run-tests.sh` to run all tests and quality gates in a single
> run.

All parts are covered by go unit tests. Run them with:

    go test -race ./...

CI/CD runs [jscpd](https://github.com/kucherenko/jscpd) a Copy/paste detector
for programming source code. Before pushing, run it locally:

    npx jscpd \
        --pattern "**/*.go" \
        --ignore "**/*_test.go" \
        --threshold 0 \
        --exitCode 1

Additionally, a python based stress test command-line utility `./dns-stress.py`
is included. Python 3.7+ and `dnspython` is required. Use
`./dns-stress.py --help`.  
Example usage:

    ./dns-stress.py --server 10.248.157.10 \
    --query "%RAND%.test.example.com" \
    --expect-content 10.111.1.154 \
    --num-requests 500000 \
    --concurrency 500
