#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ORIGINAL_DIR="$(pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
ARCHIVE_DIR="$BACKEND_DIR/builds-old"
DATE_STAMP="$(date +%m-%d-%Y)"
ARCHIVE_PATH="$ARCHIVE_DIR/backend-$DATE_STAMP"
TEMP_BUILD_PATH="$BACKEND_DIR/backend.new"

cleanup() {
  cd "$ORIGINAL_DIR"
}
trap cleanup EXIT

cd "$BACKEND_DIR"
git pull
mkdir -p "$ARCHIVE_DIR"

if [[ -e "$ARCHIVE_PATH" ]]; then
  ARCHIVE_PATH="$ARCHIVE_DIR/backend-$DATE_STAMP-$(date +%H%M%S)"
fi

if [[ -f "$BACKEND_DIR/backend" ]]; then
  mv "$BACKEND_DIR/backend" "$ARCHIVE_PATH"
fi

rm -f "$TEMP_BUILD_PATH"
go build -o "$TEMP_BUILD_PATH" ./cmd/server
mv "$TEMP_BUILD_PATH" "$BACKEND_DIR/backend"

sudo systemctl restart sfluv-backend.service
