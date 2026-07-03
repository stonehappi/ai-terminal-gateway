# AI Gateway API

An HTTP API gateway that answers natural-language requests — either as a
**business assistant** (plain-text reply) or by running **AI-generated code** in
a cloud terminal sandbox.

**Flow:** a caller sends a natural-language prompt → the gateway drives an
**agentic coding CLI** (**Claude Code**, `claude -p`; **agy**, `agy --print`; or
**OpenAI Codex**, `codex exec`) → the CLI decides how to respond:

- **answer mode** — a direct plain-text reply for questions, explanations, or
  advice (the "business assistant" path); returned as-is, nothing is executed.
- **script mode** — a self-contained script that runs in an isolated **sandbox
  terminal** (a network-less Docker container); the gateway returns the script,
  its explanation, and the captured output.

The generation backend is selectable via `LLM_PROVIDER` (`claude` by default,
`agy`, or `codex`). Auth for generation comes from that CLI's own local login —
**no `ANTHROPIC_API_KEY` is used**.

```
POST /v1/run                      generation CLI           Docker sandbox
  { "prompt": "..." }   ──▶  gateway ─────▶ generate JSON ──▶  run (no network,   ──▶  { script, stdout,
                                            {lang,script}       mem/cpu/pid caps)        stderr, exit_code }
```

## Install on Windows (no coding required)

Non-technical users don't need Go, a terminal, or `.env` editing — use the
installer:

1. Download **`ai-terminal-gateway-setup.exe`** from the
   [Releases](https://github.com/stonehappi/ai-terminal-gateway/releases) page
   and double-click it.
   - Windows may show a blue **"Windows protected your PC"** screen because the
     installer isn't code-signed yet. It's safe to run: click **More info** →
     **Run anyway**. (You can verify the download first with the published
     `.sha256` checksum: `Get-FileHash .\ai-terminal-gateway-setup.exe -Algorithm SHA256`.)
2. In the wizard, pick your AI provider (**Codex**, **Claude**, or **agy**), and
   optionally tick "log in now" and "start now".
3. Click **Install**. The wizard then:
   - generates a secure API key and a `.env` for you,
   - auto-detects Docker (uses the isolated sandbox if present, otherwise a
     local no-isolation mode),
   - registers the gateway to **start automatically every time you log in**.

When it finishes it opens **`YOUR-API-KEY.txt`** (saved in the install folder)
showing your local URL (`http://localhost:8081`) and API key, with a ready-to-use
`curl` example. You still need an account for the chosen AI CLI — the wizard can
open its login page for you. To remove it, uninstall from *Add or remove programs*.

> **Building the installer (maintainers):** requires Go and
> [Inno Setup 6](https://jrsoftware.org/isinfo.php)
> (`winget install JRSoftware.InnoSetup`). Run
> `powershell -ExecutionPolicy Bypass -File scripts\build-installer.ps1`; the
> `Setup.exe` lands in `installer\Output\`.

## Requirements (build from source)

- Go 1.23+
- One generation CLI installed and logged in, matching `LLM_PROVIDER`:
  - `claude` (default) — the **Claude Code CLI** on your PATH; verify with
    `claude -p "hi"`
  - `agy` — the **agy CLI** on your PATH; verify with `agy models`
  - `codex` — the **OpenAI Codex CLI** on your PATH, logged in via `codex login`;
    verify with `codex exec "hi"`
- Docker (for the `docker` sandbox backend — the default and recommended mode)

## Setup

```sh
cp .env.example .env        # edit as needed (no API key required)
go mod tidy
go build ./...
```

Load the env vars and run (Git Bash / macOS / Linux):

```sh
set -a && . ./.env && set +a
go run .
```

PowerShell:

```powershell
Get-Content .env | Where-Object { $_ -and $_ -notmatch '^\s*#' } | ForEach-Object {
  $k,$v = $_ -split '=',2; [Environment]::SetEnvironmentVariable($k, $v)
}
go run .
```

## API

### `GET /healthz`

Liveness check. Returns `{"status":"ok"}`.

### `POST /v1/run`

Auth: `Authorization: Bearer <one of GATEWAY_API_KEYS>` (omit if auth is disabled).

Request body:

| Field      | Type   | Required | Description                                  |
| ---------- | ------ | -------- | -------------------------------------------- |
| `prompt`   | string | yes      | The request — a question to answer or a task to perform. |
| `language` | string | no       | Force `"python"` or `"bash"`. Otherwise the model picks (and may choose to answer without code). |
| `provider` | string | no       | Pick the generation backend for this request: `"claude"`, `"agy"`, or `"codex"`. Defaults to `LLM_PROVIDER`. |

The model chooses the response `mode`; every response echoes back the
`provider` that handled it. The gateway returns one of two shapes.

**Answer mode** (business-assistant reply — no code is run):

```sh
curl -s http://localhost:8081/v1/run \
  -H "Authorization: Bearer dev-key-123" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Draft a polite reminder email for an overdue invoice"}' | jq
```

```json
{
  "provider": "claude",
  "mode": "answer",
  "answer": "Subject: Friendly reminder — invoice #1234\n\nHi ..."
}
```

**Script mode** (code generated and executed in the sandbox):

```sh
curl -s http://localhost:8081/v1/run \
  -H "Authorization: Bearer dev-key-123" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"list the first 10 prime numbers"}' | jq
```

```json
{
  "provider": "claude",
  "mode": "script",
  "language": "python",
  "script": "def is_prime(n): ...",
  "explanation": "Computes and prints the first 10 primes.",
  "execution": {
    "stdout": "2 3 5 7 11 13 17 19 23 29\n",
    "stderr": "",
    "exit_code": 0,
    "timed_out": false,
    "duration_ms": 412
  }
}
```

**Choosing a provider per request** — override `LLM_PROVIDER` per call with the
`provider` field (the CLI must be installed and logged in on the server):

```sh
curl -s http://localhost:8081/v1/run \
  -H "Authorization: Bearer dev-key-123" \
  -H "Content-Type: application/json" \
  -d '{"prompt":"list the first 10 prime numbers","provider":"codex"}' | jq
```

## Generation providers

Set `LLM_PROVIDER` to choose which CLI generates the script/answer:

- **`claude`** (default): drives `claude -p --output-format json`, reading the
  prompt from stdin and unwrapping the JSON envelope it returns. Tune with
  `CLAUDE_BIN` and `CLAUDE_MODEL`.
- **`agy`**: drives `agy --print "<prompt>"`, which prints the model's raw text.
  Tune with `AGY_BIN` and `AGY_MODEL` (see `agy models` for available models).
- **`codex`**: drives `codex exec --skip-git-repo-check "<prompt>"`, which prints
  the model's final message as raw text. Tune with `CODEX_BIN` and `CODEX_MODEL`.
  Log in first with `codex login`.

All three use the CLI's own local login for auth. Only the generation step
differs — the answer/script decision and sandbox execution are identical across
providers.

## Sandbox backends

- **`docker`** (default): each script runs in an ephemeral container with
  `--network none`, a read-only root filesystem, a small writable `/tmp`,
  dropped Linux capabilities, `no-new-privileges`, and memory/CPU/pids limits.
  The container is discarded after each run.
- **`local`**: runs the interpreter directly on the host with **no isolation**.
  Development-only convenience for machines without Docker. Never expose a
  `local`-backed gateway to untrusted callers.

## Deployment

The gateway is a single static Go binary, but it is **not** a self-contained
service: at runtime it shells out to two things on the host, so wherever it runs
must provide both:

1. The generation CLI (`claude` or `agy`) **on `PATH` and already logged in** —
   auth is the CLI's own local session, so you log in interactively once on the
   host (`claude` / `agy`) rather than passing an API key.
2. The **Docker daemon** (for `SANDBOX_BACKEND=docker`), reachable by the user
   the gateway runs as.

Because of (1), the simplest and recommended topology is to run the gateway
**directly on a host/VM** rather than in a minimal container.

### 1. Build a release binary

```sh
# On the target's OS/arch, or cross-compile:
CGO_ENABLED=0 go build -o ai-gateway-api .
# Cross-compile examples:
GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -o ai-gateway-api .
GOOS=windows GOARCH=amd64 go build -o ai-gateway-api.exe .
```

### 2. Configure

Copy `.env.example` to `.env` and set at least:

- `GATEWAY_API_KEYS` — **required in production**; never leave auth disabled.
- `LLM_PROVIDER` and the matching `*_BIN` / `*_MODEL`.
- `SANDBOX_BACKEND=docker` (keep this for any shared deployment).
- `PORT` — the local port the process listens on.

### 3. Run as a service

**Linux (systemd)** — pre-pull the sandbox images so the first request is fast,
then run the binary as a login-capable service user that has the CLI logged in
and belongs to the `docker` group:

```ini
# /etc/systemd/system/ai-gateway.service
[Unit]
Description=AI Gateway API
After=network-online.target docker.service
Wants=network-online.target

[Service]
User=aigateway
WorkingDirectory=/opt/ai-gateway
EnvironmentFile=/opt/ai-gateway/.env
ExecStart=/opt/ai-gateway/ai-gateway-api
Restart=on-failure
# Harden the process (the sandbox still isolates generated code separately):
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/ai-gateway

[Install]
WantedBy=multi-user.target
```

```sh
docker pull python:3.12-slim && docker pull bash:5
sudo systemctl daemon-reload
sudo systemctl enable --now ai-gateway
journalctl -u ai-gateway -f          # tail the JSON logs
```

**Windows** — run `run.bat` (loads `.env`, then `go run .`) for a quick start,
or register the built `ai-gateway-api.exe` as a service with a tool like
[NSSM](https://nssm.cc/) / Task Scheduler so it survives logout. The service
account must have Docker Desktop running and the generation CLI logged in.

**Windows auto-start at logon** — the `scripts/` folder registers a per-user
scheduled task (no admin needed) that starts the gateway every time you log in
and restarts it if it crashes. It runs as your user, so your generation-CLI
login and Docker Desktop are available.

```powershell
# Install (once). Create a .env first — GATEWAY_API_KEYS especially.
powershell -ExecutionPolicy Bypass -File scripts\install-autostart.ps1

Start-ScheduledTask -TaskName AITerminalGateway   # start now, without logging out
Stop-ScheduledTask  -TaskName AITerminalGateway   # stop it
powershell -File scripts\uninstall-autostart.ps1  # remove auto-start
```

The task runs `scripts\gateway-autostart.ps1`, which loads `.env`, builds
`ai-gateway-api.exe` if missing, and launches it. **Set `GATEWAY_API_KEYS` in
`.env` before enabling auto-start** — otherwise the gateway comes up
unauthenticated on port 8081.

### 4. Put it behind TLS + a reverse proxy

The gateway serves plain HTTP and does no rate limiting. In production, front it
with nginx / Caddy / a cloud load balancer that terminates TLS and forwards to
`127.0.0.1:$PORT`. Bind the gateway to localhost and let only the proxy reach it.

```nginx
server {
    listen 443 ssl;
    server_name gateway.example.com;
    # ssl_certificate ...; ssl_certificate_key ...;
    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_read_timeout 360s;   # generation can take minutes
    }
}
```

### 5. Health checks

Point your orchestrator/load balancer liveness probe at `GET /healthz`
(unauthenticated, returns `{"status":"ok"}`).

### Containerizing the gateway (advanced)

You *can* run the gateway itself in a container, but you must solve both host
dependencies: bake the generation CLI into the image **with a valid login baked
in or mounted** (e.g. mount the CLI's credential/config dir), and give the
container access to a Docker daemon by mounting the host socket
(`-v /var/run/docker.sock:/var/run/docker.sock`). Note that the sandbox
containers are then **siblings** on the host daemon, not nested — the
`--network none` / resource caps still apply. Weigh the socket mount's blast
radius against the isolation you gain.

## Security notes

- AI-generated code is untrusted. Keep `SANDBOX_BACKEND=docker` in any shared or
  production deployment, and always set `GATEWAY_API_KEYS`.
- The Docker image is pulled on first use; pre-pull with
  `docker pull python:3.12-slim` and `docker pull bash:5` to avoid a slow first
  request.
- On Windows, run the `docker` backend via Docker Desktop; the temp-dir bind
  mount requires the drive to be shared with Docker Desktop.

## Usage & third-party terms

This tool drives the **Claude Code**, **agy**, and **OpenAI Codex** CLIs using
**your own local login** for each. You are responsible for complying with each
CLI's Terms of Service and Usage Policy — in particular, **do not use it to
resell or proxy access to those services for third parties**. Use your own
account, for your own use.

Publishing or forking this code is fine; how you *operate* it is on you. Check
the authoritative sources yourself:

- **Anthropic** (Claude Code) — Usage Policy, Consumer Terms, and Commercial
  Terms (search "Anthropic usage policy" / "Anthropic commercial terms").
- **OpenAI** (Codex) — Usage Policies and the applicable API/ChatGPT terms for
  the login you use (search "OpenAI usage policies" / "OpenAI terms of use").
- **agy** — its own vendor terms.

This project is not affiliated with, endorsed by, or sponsored by Anthropic,
OpenAI, or the agy vendor. Product names are used only to identify the tools
this gateway integrates with.
