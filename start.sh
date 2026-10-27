#!/bin/sh
set -eu

(
	cd backend
	exec go run ./cmd/api
) &
api_pid=$!
trap 'kill "$api_pid" 2>/dev/null || true' EXIT INT TERM

npm run dev
