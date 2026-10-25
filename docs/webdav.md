# WebDAV Server

`camel serve` exposes cloud storage as a WebDAV endpoint, allowing any WebDAV client to browse, upload, download, and manage files.

## Quick Start

```bash
# Start server with saved credentials
camel serve lt

# Custom port + inline token (no login required)
camel serve lt --addr :9000 --token YOUR_ACCESS_TOKEN

# With Basic auth protection
camel serve lt --user myphone --token YOUR_TOKEN
```

The server defaults to `:8080`. Any standard WebDAV client can connect.

## Supported Providers

| Provider | Identifier | Description |
|----------|-----------|-------------|
| Liantong (联通云盘) | `lt` | China Unicom Cloud Disk — full support |
| Baidu Netdisk | `baidu` | Planned |

## Command-line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-a, --addr` | `:8080` | Listen address (e.g. `:9000`, `127.0.0.1:8080`) |
| `-t, --token` | *(from login)* | Access token; overrides credential file |
| `-u, --user` | *(from login)* | Username for WebDAV Basic auth |
| `--tls-cert` | *(none)* | TLS certificate path; enables HTTPS |
| `--tls-key` | *(none)* | TLS private key path; enables HTTPS |

## Supported WebDAV Methods

| Method | Operation | Notes |
|--------|-----------|-------|
| `PROPFIND` | List directory / get properties | Depth `0` and `1`; `Depth: infinity` is rejected with `403` and `<D:propfind-finite-depth/>`. Supports `allprop`, `propname` and named properties. |
| `PROPPATCH` | Set/remove dead properties | Properties are kept in memory (see limitations). |
| `GET` | Download file | Streams file content; supports `Range` requests. |
| `HEAD` | Get file metadata | Returns `Content-Length`/`Content-Type` without downloading. |
| `PUT` | Upload file | Buffers the body, then uploads via the provider API. Returns `201` for a new file, `204` when overwriting. |
| `DELETE` | Delete file or directory | Retries until the provider's listing confirms the resource is gone. |
| `MKCOL` | Create directory | `405` if the resource exists, `409` if the parent is missing. |
| `MOVE` | Move or rename | Requires `Destination`; `201` when created, `204` when overwriting. |
| `COPY` | Copy file or collection | Requires `Destination`; honours `Depth` (`0`, `1`, `infinity`); `201`/`204` as for MOVE. |
| `OPTIONS` | Capabilities | Advertises `DAV: 1, 3` and the allowed methods. |

### Limitations

- **Class 1 only.** `LOCK`/`UNLOCK` (WebDAV class 2) are not implemented. The
  litmus `locks` suite is therefore skipped; clients that require locking
  (e.g. some Microsoft Office save flows) are not supported.
- **Dead properties are in-memory.** `PROPPATCH` values are carried across
  `MOVE`/`COPY`/`DELETE` and honoured by `PROPFIND` while the server process is
  running, but they are lost when it restarts.
- **Zero-byte uploads** are rejected with `501` because the provider's upload
  API rejects empty files.
- The provider's listing API does not expose modification times, so
  `getlastmodified`/`creationdate` are the time of the current request.

## Testing with litmus

[litmus](https://github.com/notroj/litmus) is the reference WebDAV compliance
test suite. The repository includes a harness that builds the binary, starts a
throw-away server and runs the suites:

```bash
# Install once (Debian/Ubuntu): apt install litmus
./scripts/litmus.sh                          # basic copymove props http
TESTS="basic" ./scripts/litmus.sh            # a single suite
ADDR=127.0.0.1:9100 ./scripts/litmus.sh      # different port
```

Logs (including a per-request server log) are written to `.litmus/`. Set
`CAMEL_WEBDAV_DEBUG=1` when running `camel serve` manually to log every
request with its status and latency.

## Authentication

By default the server runs without authentication. Use flags for protection:

```bash
# Basic auth using stored credentials
camel serve lt --user $(cat ~/.camel/lt-phone 2>/dev/null)

# Or provide both inline
camel serve lt -u myphone -t your_token_here
```

With Basic auth enabled, clients must send `Authorization: Basic <base64>` headers.

## Example Clients

### rclone mount

```ini
# ~/.config/rclone/rclone.conf
[mycloud]
type = webdav
url = http://127.0.0.1:8080
vendor = other
user = myphone
pass = your_token_here
```

```bash
rclone mount mycloud:/ ~/mnt/cloud &
```

### cadaver (CLI client)

```bash
cadaver http://127.0.0.1:8080
```

### FileZilla (GUI FTP/WebDAV client)

Connect via SFTP-style WebDAV URL: `http://127.0.0.1:8080`

### macOS Finder

Go → Connect to Server → `http://127.0.0.1:8080`

## Security Notes

- The server does **not** encrypt traffic unless `--tls-cert` and `--tls-key` are provided. For production use, always enable TLS or put the server behind a reverse proxy (nginx, Caddy).
- Tokens are passed through to the provider but never persisted by the WebDAV server itself.
- Zero-byte uploads are rejected (501 Not Implemented), consistent with the provider's limitations.
