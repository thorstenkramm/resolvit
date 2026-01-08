# Filtering

## Overview

Resolvit can block domains using curated blocklists and a local allowlist.
Blocked queries return NXDOMAIN, while local records are always served even if a
domain also appears in a blocklist.

## Configuration

Filtering is configured only through `/etc/resolvit/resolvit.conf` (TOML syntax).
The sample file `resolvit.conf.example` shows the full set of options and list
blocks.

### Core settings

- `filtering.enabled`: Turns filtering on or off. Default: `false`.
- `filtering.block_subdomains`: When `true`, `example.com` blocks
  `foo.example.com`. Default: `false`.
- `filtering.cache_dir`: Cache directory for downloaded lists. Default:
  `/var/cache/resolvit`.
- `filtering.update_interval`: Refresh interval for list downloads. Default:
  `24h`.
- `filtering.blocked_log`: Optional log for blocked queries. Default: empty
  (disabled).

### Allowlist

Use `filtering.allowlist.path` to point at a plain text file, one domain per
line. Comments with `#`, `//`, and `;` are supported. Wildcards such as
`*.example.com` are allowed.

### Custom lists

Use `filtering.custom.list` to add URL or file-based lists. Each entry is a URL
or a local file path. Comments and wildcards follow the same rules as the
built-in lists.

### Authentication

Paid lists may require credentials. Configure authentication inside the list
block:

```toml
[filtering.spamhaus_dbl]
#enabled = true
#username = "account-id"
#password = "secret"

[filtering.surbl]
#enabled = true
#token = "abc123"
#header = "Authorization"
#scheme = "Bearer"
#url = "https://provider.example/list?token=abc123"
```

## Built-in list catalog

Enable list blocks under `[filtering.<list_id>]` with `enabled = true`.

### Recommended for business defaults

- `blocklistproject_malware`: Malware-hosting domains.
- `blocklistproject_phishing`: Phishing domains.
- `blocklistproject_scam`: Scam domains.
- `blocklistproject_porn`: Adult content domains.

### Optional lists (ads/tracking and alternatives)

- `blocklistproject_ads`: Advertising domains (optional for ad-friendly
  environments).
- `oisd_basic`: Ads and tracking domains.
- `adguard_dns`: Ads and tracking domains.
- `stevenblack_adult`: Adult-only list without ad blocking.

### Paid lists

- `spamhaus_dbl`: Domain Blocklist (DBL). Paid plan, basic auth required.
- `surbl`: Domain reputation list. Paid plan, token or URL auth required.

## Update and cache behavior

On startup, resolvit downloads enabled lists. If a download fails, it logs the
error and uses the last cached list if available. Otherwise that list is
skipped and resolvit continues to start. The cache directory is created if
missing. If it cannot be created, caching is disabled and filtering continues
without persistence.

## Logging and error handling

- `filtering.blocked_log` is off by default for privacy. When set, each blocked
  query is logged.
- `logging.blocklist_error_limit` caps per-list parse errors and emits a
  summary when exceeded.
- Invalid or malicious list entries are logged and ignored. Parsing continues
  with valid entries.
