# resolvit

A DNS server that allows you to resolve specific DNS records locally while forwarding all other
requests to upstream DNS servers.

## Features

- Local resolution of A and CNAME records
- No need to create zones
- DNS forwarding to upstream servers
- DNS response caching
- Configurable via command line flags
- Supports wildcards for A and CNAME records

Local records are read to memory on service start. Changes to the records file require
a service restart.

Records remain in the cache according to the TTL given by the upstream server.  
The cache is in memory. It gets lost on service restart.

## Usage

Start the DNS server:

```bash
resolvit --listen 127.0.0.1:5300 --upstream 8.8.8.8
```

Configuration Options:

```
--listen: Listen address for DNS server (default "127.0.0.1:5300")
--upstream: Upstream DNS server (can specify multiple, default "9.9.9.9:53")
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

## Limitations

Only A and CNAME records are supported for local resolution
All other record types are forwarded to upstream DNS servers

## Example Usage

Create a records file:

    echo "local.dev A 127.0.0.1" > records.txt
    echo "www.local.dev CNAME local.dev" >> records.txt

Start the server with local records:

    overridedns --listen 127.0.0.1:5300 --upstream 8.8.8.8:53 --resolve-from records.txt

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
Type=simple
ExecStart=/usr/local/bin/resolvit --listen 127.0.0.1:5300 --upstream 8.8.8.8:53 --resolve-from /etc/overridedns/records.txt
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
User=resolvit
Group=resolvit

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

    systemctl enable resolvit
    systemctl start resolvit