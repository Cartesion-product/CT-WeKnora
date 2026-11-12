#!/usr/bin/env bash
set -euo pipefail

cd /app
cp -a /source/docreader/. docreader/
bash docreader/scripts/generate_proto.sh
exec /app/.venv/bin/python -m docreader.container_entrypoint /app/.venv/bin/python -m docreader.main
