#!/usr/bin/env bash
set -euo pipefail

REPO_URL="https://raw.githubusercontent.com/gandarez/rinha-de-backend-2026/main/resources"
DEST="${RESOURCES_DIR:-./resources}"

mkdir -p "$DEST"

echo "Downloading normalization.json..."
curl -fsSL "$REPO_URL/normalization.json" -o "$DEST/normalization.json"

echo "Downloading mcc_risk.json..."
curl -fsSL "$REPO_URL/mcc_risk.json" -o "$DEST/mcc_risk.json"

echo "Downloading references.json.gz (~50 MB)..."
curl -fsSL "$REPO_URL/references.json.gz" -o "$DEST/references.json.gz"

echo "Done. Files in $DEST:"
ls -lh "$DEST/"
