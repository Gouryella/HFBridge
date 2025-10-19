# HFBridge - Hugging Face Proxy with Web UI

[English](./README.md) | [中文](./README.zh.md)

HFBridge is an all-in-one Hugging Face proxy service with a built-in web dashboard. It provides transparent request forwarding, Large File Storage (LFS) link rewriting, and real-time usage analytics so that teams working behind firewalls or inside restricted networks can keep using Hugging Face resources.

## Features

- Transparent proxying for Hugging Face repositories, models, and files with a configurable upstream origin
- Automatic injection of a Hugging Face access token for private resources
- LFS `batch` response rewriting so large files are fetched via the proxy's `/lfs/content` endpoint
- Usage and bandwidth tracking persisted in SQLite with a live dashboard
- Integrated web UI for monitoring usage, repository statistics, and traffic trends
- Single-container deployment with Docker or a self-contained binary build

## Quick Start

The repo ships with a ready-to-run Compose stack that pulls the published `gouryella/hfbridge:latest` image.

### Prerequisites

- Docker Engine 20.10+ with the `docker compose` plugin (or Docker Desktop)

### Launch with Docker Compose

1. Clone this repository and adjust environment variables in `docker-compose.yml` if needed (for example `DEFAULT_UPSTREAM`, `HF_TOKEN` or `PROXY_ORIGIN`).
2. Start the service:

   ```bash
   docker compose up -d
   ```

3. Visit `http://localhost:8080` to open the web dashboard.
4. Follow logs when troubleshooting:

   ```bash
   docker compose logs -f hfbridge
   ```

5. Stop the stack when you are done:

   ```bash
   docker compose down
   ```

By default, the container persists its SQLite database to `./data` on the host. Adjust the volume section in `docker-compose.yml` if you need a different location.

### Optional: Build and Run Manually

If you prefer to build the image yourself:

```bash
docker build -t hfbridge .
docker run -p 8080:8080 -v $(pwd)/data:/data hfbridge
```

For local binary builds, see the Development section.

## Usage

### Web Dashboard

Open `http://localhost:8080` to inspect live usage metrics, repository rankings, and traffic charts.

### Clone Repositories Through the Proxy

```bash
# Proxy a public repository
git clone http://localhost:8080/google/bert

# Large file downloads are also rewritten through the proxy
git clone http://localhost:8080/runwayml/stable-diffusion-v1.5
```

### Use the Proxy with Hugging Face Tools

Once the service is running, the proxy is available at `http://<host>:8080`. Point your tooling to this base URL to route traffic through HFBridge:

- **huggingface-hub / transformers (Python)**  
  Configure the endpoint before using `huggingface-cli` or loading models:

  ```bash
  export HF_ENDPOINT=http://localhost:8080
  export HF_TOKEN=hf_your_token_if_needed
  huggingface-cli whoami
  ```

  Your existing scripts that rely on `huggingface_hub` or `transformers` will now download models and datasets via the proxy.

- **Git clients**  
  To transparently clone any repository through HFBridge, set a Git rewrite rule once:

  ```bash
  git config --global url."http://localhost:8080/".insteadOf "https://huggingface.co/"
  ```

  Afterwards, any command that targets `https://huggingface.co/...` will automatically use the proxy (including Git LFS).

- **Direct HTTP calls**  
  Replace `https://huggingface.co` with `http://localhost:8080` in automation scripts. For example:

  ```bash
  curl http://localhost:8080/api/models/gpt2
  ```

### API Endpoints

```bash
# Health check
curl http://localhost:8080/v1/healthz

# Current usage metrics (JSON)
curl http://localhost:8080/v1/usage

# Live usage stream (SSE)
curl http://localhost:8080/v1/usage/stream
```

## Configuration

All settings are available via environment variables.

| Variable           | Required | Default value                                                            | Description |
|--------------------|----------|---------------------------------------------------------------------------|-------------|
| `BIND_ADDR`        | No       | `:8080`                                                                  | HTTP listen address |
| `DEFAULT_UPSTREAM` | No       | `https://huggingface.co`                                                 | Default upstream used when a request path is relative |
| `PROXY_ORIGIN`     | No       | Auto-detected                                                            | Public-facing base URL used when rewriting LFS links; override if auto-detection is wrong |
| `ALLOW_HOSTS`      | No       | `huggingface.co,cdn-lfs.huggingface.co,*.huggingface.co,*.hf.co`         | Comma-separated list of upstream hosts allowed for proxying and LFS downloads |
| `HF_TOKEN`         | No       | empty                                                                    | Hugging Face access token for private resources |
| `DB_PATH`          | No       | `/data/hfbridge.db`                                                      | SQLite file storing request logs and stats |

## Architecture

HFBridge is delivered as a single binary that embeds both the backend proxy and the web UI.

- **Frontend**: Next.js dashboard compiled to static assets and embedded into the Go binary.
- **Backend**: Go reverse proxy server serving API endpoints and static files.
- **Database**: SQLite file containing request logs and aggregated usage data.

Routing overview:

- `/` serves the dashboard.
- `/_next/*`, `/icons/*`, `/favicon.ico` serve static assets.
- `/v1/usage`, `/v1/usage/stream`, `/v1/healthz` expose API endpoints.
- Any other path (e.g. `/google/bert`) is proxied to the configured upstream.

## Development

```
hfbridge/
├── frontend/          # Next.js UI
│   ├── app/           # App Router pages
│   ├── components/    # React components
│   └── out/           # Static build output
├── backend/           # Go backend
│   ├── cmd/hfbridge/  # Main entry point
│   └── internal/      # Internal packages
│       └── server/    # HTTP server and embedded assets
├── Dockerfile         # Multi-stage build
└── docker-compose.yml
```

### Local Development Workflow

```bash
# Frontend development (hot reload)
cd frontend
pnpm install
pnpm dev

# Backend development (requires a built frontend once)
cd backend
go run ./cmd/hfbridge/
```

To ship a self-contained binary:

1. Build the frontend (`pnpm build`) and copy the assets into `backend/internal/server/static`.
2. Compile the backend with `go build -o hfbridge ./cmd/hfbridge/`.
3. Run `./hfbridge` and visit `http://localhost:8080`.

## License

GNU GPLv3 (see `LICENSE`)
