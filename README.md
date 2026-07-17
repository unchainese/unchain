# Unchain

Unchain is a lightweight Go-based VLESS over WebSocket proxy project with support for:

- VLESS over WebSocket proxy server
- TCP and UDP forwarding
- multiple UUID user access control
- optional traffic usage metering
- systemd service installation
- Docker image packaging

## Key Features

- **Protocol**: VLESS over WebSocket
- **Endpoints**: `/wsv/{uid}`, `/sub/{uid}`, `/`
- **Configuration**: `.env` file and environment variables
- **User Management**: `ALLOW_USERS` accepts multiple UUIDs
- **Traffic Metering**: enabled via `ENABLE_DATA_USAGE_METERING=true`
- **Modes**: `run`, `install`, `client`

## Project Structure

```text
.
├── Dockerfile
├── LICENSE
├── README.md
├── app.go
├── app_ping.go
├── app_sub.go
├── app_ws_vless.go
├── config.go
├── config_util.go
├── example.env
├── go.mod
├── go.sum
├── logger.go
├── main.go
├── socks5.go
├── unchain
├── unchain.service
└── vless.go
```

## Quick Start

### 1. Clone the repository

```bash
git clone https://github.com/unchainese/unchain.git
cd unchain
```

### 2. Install dependencies

```bash
go mod download
```

### 3. Configure environment variables

Copy `example.env` to `.env` and modify values as needed:

```bash
cp example.env .env
```

### 4. Run the service

```bash
go run main.go run
```

## Commands

- `go run main.go run` - start the proxy server
- `go run main.go install` - install systemd service (if `systemctl` is available)
- `go run main.go client` - start the built-in SOCKS5 client
- `go run main.go help` - display help information

## Configuration

Unchain reads configuration from `.env` or environment variables. The following variables map to the `Config` struct in `config.go`:

| Variable                     | Default                                                                     | Description                                                          |
| ---------------------------- | --------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `APP_HOST`                   | `svr.libragen.unchain`                                                      | Host or host list for subscription addresses, comma separated        |
| `APP_PORT`                   | `8880`                                                                      | HTTP listen port                                                     |
| `REGISTER_URL`               | `https://unchain.libragen.cn/api/node`                                      | Control server registration URL; leave empty to disable registration |
| `REGISTER_TOKEN`             | `unchain people from censorship and surveillance`                           | Control server token                                                 |
| `ALLOW_USERS`                | `903bcd04-79e7-429c-bf0c-0456c7de9cdc,903bcd04-79e7-429c-bf0c-0456c7de9cd1` | Allowed UUID list                                                    |
| `LOG_FILE`                   | ``                                                                          | Log file path; empty means stdout                                    |
| `DEBUG_LEVEL`                | `DEBUG`                                                                     | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR`                          |
| `INTERVAL_SECOND`            | `3600`                                                                      | Interval in seconds to push node info to `REGISTER_URL`              |
| `GIT_HASH`                   | ``                                                                          | Optional build git hash                                              |
| `BUILD_TIME`                 | ``                                                                          | Optional build timestamp                                             |
| `RUN_AT`                     | ``                                                                          | Optional runtime timestamp                                           |
| `ENABLE_DATA_USAGE_METERING` | `true`                                                                      | Enable traffic metering                                              |
| `BUFFER_SIZE`                | `8192`                                                                      | Buffer size for WebSocket and TCP/UDP reads                          |

> Note: This project uses `.env` / environment variables for configuration and does not load `config.toml`.

## Runtime Behavior

- On startup, the node registers to `REGISTER_URL` if configured.
- The application prints VLESS subscription examples.
- `/sub/{uid}` returns VLESS subscription URLs.
- `/wsv/{uid}` handles VLESS WebSocket connections.
- `/` returns status and traffic statistics.
- `/debug/pprof/...` provides pprof profiling endpoints.

## HTTP Endpoints

| Path               | Description                    |
| ------------------ | ------------------------------ |
| `/wsv/{uid}`       | VLESS WebSocket entry point    |
| `/sub/{uid}`       | Subscription URL generator     |
| `/`                | Health check and runtime stats |
| `/debug/pprof/...` | pprof profiling endpoints      |

## VLESS Subscription URL

The subscription URL is generated from `APP_HOST` and looks like:

```text
vless://<UUID>@<host>:<port>?encryption=none&allowInsecure=1&type=ws&path=/wsv/<UUID>?ed=2560#<host>
```

## SOCKS5 Client

Running `go run main.go client` starts a built-in SOCKS5 server.

The client uses a fixed `wsURL` and `vlessUUID` defined in `socks5.go` to forward traffic to a remote WebSocket server. It is intended for testing or experimental use.

## Docker Usage

### Build the image

```bash
docker build -t unchain .
```

### Run the container

```bash
docker run -d --name unchain \
  -p 8880:8880 \
  -e APP_HOST=svr.libragen.unchain \
  -e APP_PORT=8880 \
  -e REGISTER_URL=https://unchain.libragen.cn/api/node \
  -e ALLOW_USERS=903bcd04-79e7-429c-bf0c-0456c7de9cdc \
  unchain
```

## Build Binary

```bash
go build -o unchain main.go
./unchain run
```

## Code Overview

- `main.go` - entry point and subcommand parsing
- `config_util.go` - environment loading and config mapping
- `app.go` - HTTP server initialization, run loop, and graceful shutdown
- `app_ws_vless.go` - VLESS over WebSocket forwarding logic
- `app_sub.go` - subscription URL generation
- `app_ping.go` - health check and runtime stats endpoint
- `socks5.go` - SOCKS5 client implementation
- `vless.go` - VLESS payload encode/decode logic
- `logger.go` - logger setup

## Recommendations

- Use a reachable domain in `APP_HOST`.
- Ensure `ALLOW_USERS` contains valid UUIDs.
- Leave `REGISTER_URL` empty if you do not want registration.
- For production, deploy behind a reverse proxy with TLS.

## Dependencies

- `github.com/google/uuid`
- `github.com/gorilla/websocket`
- `github.com/joho/godotenv`

## License

Apache License 2.0. See `LICENSE` for details.
