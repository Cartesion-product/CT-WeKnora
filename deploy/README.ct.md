# CT-WeKnora deployment

This deployment uses the existing ParadeDB, Redis, Neo4j, Ollama, and MinIO containers. The Compose file starts only WeKnora app, frontend, and DocReader. Its source bind mounts explicitly use `/home/chitong_application/CT-WeKnora`; copy the complete repository there and run these commands from that directory.

## Prerequisites

- Docker Engine with the Compose v2 plugin. The current Windows workspace does not have the `docker` command, so this configuration has not been boot-tested here.
- Copy this repository, including the ignored `.env`, to the Docker host. Keep `.env` out of Git and backups that are shared publicly.
- Ensure the host gateway can reach PostgreSQL on `15432`, Neo4j on `7687`, Ollama on `11434`, and MinIO on `9000`. The Redis server at `192.168.2.205:6379` must permit connections from the Docker host. If any dependency runs on a different machine, change its corresponding host in `.env`.
- Confirm the ParadeDB container provides `pgvector` and `pg_search`, and that database `WeKnora` is accessible by user `postgres`. Existing volumes are left untouched.
- Pull `bge-m3` into the Ollama instance and ensure Ollama listens on an address reachable through `host.docker.internal` from containers. A service bound only to `127.0.0.1` on a Linux Docker host cannot be reached through that gateway address.
- Keep `frozenlab/qwen3-asr:1.7b` installed in Ollama. This model needs an Ollama release with Qwen3-ASR audio projector support. The app image includes ffmpeg for audio conversion.
- MinIO must be reachable using the credentials in `.env`. WeKnora creates bucket `weknora` if it does not exist.

## Start

```bash
docker compose -f docker-compose.ct.yml config --quiet
docker compose -f docker-compose.ct.yml build
docker compose -f docker-compose.ct.yml up -d
docker compose -f docker-compose.ct.yml ps
docker compose -f docker-compose.ct.yml logs -f app
```

The image build downloads Go and npm dependencies. The defaults use `https://goproxy.cn` and `https://registry.npmmirror.com` because the Docker host may not reach the upstream registries. Override `GOPROXY` or `NPM_REGISTRY` in `.env` if your server uses different mirrors.

Visit `http://192.168.2.55:8900`. On first boot, register `shin@qcznai.com`; the configured bootstrap admin email promotes that account at application startup. Restart `app` after registration, then set `DISABLE_REGISTRATION=true` in `.env` and recreate the app:

The read-only graph portal is at `http://192.168.2.55:8900/graphs`. It uses the normal WeKnora account and server-side knowledge-base permissions. It lists only accessible knowledge bases with Wiki indexing enabled and opens their Wiki page-link graph. A direct link to `/graphs/<knowledge-base-id>` opens a particular graph after login. This is the Wiki link graph, not the Neo4j entity-relationship extraction graph. External systems may link to this page or embed it in an iframe. For a cross-origin iframe, set `GRAPH_FRAME_ORIGINS` in `.env` to the exact parent origin, for example `GRAPH_FRAME_ORIGINS=https://portal.example.com`, then rebuild and recreate `frontend`; multiple origins can be space-separated. The default permits same-origin framing only. The parent origin must use HTTPS if the parent page uses HTTPS. Browser third-party storage policy can still affect login inside an iframe, so test the target browsers before relying on this integration.

To preview local frontend changes from other devices on the LAN, build the frontend and run this from `frontend/` in PowerShell. The preview binds to all local interfaces and proxies API calls to the deployed server:

```powershell
$env:VITE_DEV_PROXY_TARGET = 'http://192.168.2.55:8900'
npm run build
npm run preview -- --host 0.0.0.0 --port 5184 --strictPort
```

Open `http://192.168.1.128:5184/graphs` from a device that can reach the Windows host. Allow inbound TCP 5184 in the Windows firewall if another device cannot connect.

```bash
docker compose -f docker-compose.ct.yml restart app
# After changing .env:
docker compose -f docker-compose.ct.yml up -d --force-recreate app
```

The first app start runs database migrations. Back up the database before upgrading to a newer fork revision. Keep `SYSTEM_AES_KEY` stable: changing it makes encrypted model credentials unreadable.

## Source updates

The app reads the repository at `/source` and recompiles the Go binary on every container start. Go dependencies are downloaded into the image during `docker compose build` using `GOPROXY` (default `https://goproxy.cn`); runtime compilation is offline. Frontend dependencies are also installed during image build, then its mounted source is compiled before Nginx starts. DocReader copies mounted Python code and regenerates protobuf bindings. After pulling new source onto the Docker host:

```bash
git pull --ff-only
docker compose -f docker-compose.ct.yml restart docreader app frontend
docker compose -f docker-compose.ct.yml ps
```

The restart command may return before Go and frontend compilation complete. Watch `docker compose -f docker-compose.ct.yml logs -f app frontend docreader`. Changes to `go.mod`, `go.sum`, `frontend/package-lock.json`, or Python dependencies require rebuilding the source images and recreating services. A changed upstream runtime image also requires a rebuild. Source mode intentionally trades longer restarts and larger images for bind-mounted code updates.

## Model and registration limits

`config/builtin_models.yaml` registers the remote LLM for both chat and vision (as separate KnowledgeQA and VLLM entries), Ollama `bge-m3` embedding (1024 dimensions), the vLLM `bge-reranker-v2-m3` reranker, and Ollama `frozenlab/qwen3-asr:1.7b` speech recognition model on app startup. The VLLM and rerank entries are deployment defaults. The rerank Base URL is the vLLM `/v1` root; WeKnora appends `/rerank`. The ASR adapter converts uploads to 16 kHz mono WAV, sends 30-second chunks through Ollama `/api/chat`, and joins the transcript. In Settings > Default Settings, confirm the visual, rerank, and speech defaults. For microphone input in a custom agent, also enable audio upload and select the ASR model in that agent's settings.

To deploy the rerank model, update `.env` and `config/builtin_models.yaml` on the server, then recreate the app so it sees the current environment. If the ASR adapter changes are not on the server yet, also update `internal/models/asr/asr.go` and `internal/models/asr/ollama.go`; the app recompiles the mounted Go source at startup:

```bash
docker compose -f docker-compose.ct.yml up -d --force-recreate app
docker compose -f docker-compose.ct.yml logs -f app
```

The reranker uses vLLM at `http://192.168.2.55:8009/v1/rerank`; its name must match the `id` reported by that instance's `/v1/models`. The deployment's SSRF whitelist includes the RFC1918 LAN ranges (`10.0.0.0/8`, `172.16.0.0/12`, and `192.168.0.0/16`) so other private-network model endpoints can be configured too. This permits outbound requests from WeKnora to those LAN ranges, subject to normal network and firewall rules. Before using the model for search, verify that vLLM returns scores in the 0..1 range; otherwise configure the model's score scale as `logit` so WeKnora normalizes it before applying rerank thresholds.

WeKnora does not consume `SIGNUP_CODE` or `ALLOWED_HOSTS`. Registration is temporarily open for first admin creation; use the application's invitation-based registration mode or disable registration afterward. The frontend listens on host port `8900`; restrict access with the host firewall or a reverse proxy as needed. The supplied AI Station secret is used as `JWT_SECRET`, and the supplied public URL is mapped to `FRONTEND_BASE_URL` and `APP_EXTERNAL_URL`.
