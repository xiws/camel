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
| `PROPFIND` | List directory / get properties | Supports Depth 0 and 1; infinite depth returns error |
| `GET` | Download file | Streams file content |
| `HEAD` | Get file metadata | Returns Content-Length and Content-Type only |
| `PUT` | Upload file | Creates temp copy then uploads via provider API |
| `DELETE` | Delete file or directory | Single-resource deletion |
| `MKCOL` | Create directory | Fails if directory already exists |
| `MOVE` | Move or rename | Requires `Destination` header |
| `COPY` | Copy file | Requires `Destination` header; zero-byte copies unsupported |

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
