#!/usr/bin/env bash
set -euo pipefail

mkdir -p /work/frontend /usr/share/nginx/html
tar -C /source/frontend --exclude=node_modules --exclude=dist -cf - . | tar -C /work/frontend -xf -
cd /work/frontend
cp -a /opt/weknora/frontend/node_modules ./node_modules
VITE_IS_DOCKER=true npm run build
cp -a dist/. /usr/share/nginx/html/
exec sh /usr/local/bin/weknora-frontend-entrypoint
