#!/usr/bin/env bash
# LET_IT_RIP.sh — the full gate: set up, regenerate, build, test everything.
# Chains: LET_IT_RIP.sh -> test.sh -> build.sh -> setup.sh. Run this before
# every commit/push.
set -euo pipefail
cd "$(dirname "$0")"
./test.sh
echo "[rip] OK"
