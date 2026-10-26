#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
deploy_env_file=${1:-"$HOME/.config/advertiser/production.env"}

if [ "$#" -gt 1 ]; then
    echo "Usage: $0 [path/to/.env]" >&2
    exit 1
fi
if [ ! -f "$deploy_env_file" ]; then
    echo "Missing environment file: $deploy_env_file. Create it from .env.example and fill in the secrets once." >&2
    exit 1
fi
# Use an absolute path so POSIX sh never searches PATH when sourcing the file.
deploy_env_file=$(CDPATH= cd -- "$(dirname -- "$deploy_env_file")" && pwd)/$(basename -- "$deploy_env_file")
set -a
. "$deploy_env_file"
set +a
: "${TLGRM_BOT_TOKEN:?Set TLGRM_BOT_TOKEN in the environment file}"
: "${GCP_CREDS_JSON_BASE64:?Set GCP_CREDS_JSON_BASE64 in the environment file}"
: "${FIREBASE_DATABASE_URL:?Set FIREBASE_DATABASE_URL in the environment file}"

exec doctl serverless deploy "$project_dir" --env "$deploy_env_file" --remote-build
