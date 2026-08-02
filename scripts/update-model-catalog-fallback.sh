#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_URL="${MODELS_DEV_CATALOG_URL:-https://models.dev/catalog.json}"
TARGET="${ROOT_DIR}/controller/data/model-catalog/models-dev-fallback.json"
TMP_FILE="$(mktemp)"

cleanup() {
  rm -f "${TMP_FILE}"
}
trap cleanup EXIT

curl -fsSL --retry 3 --connect-timeout 10 --max-time 120 "${SOURCE_URL}" -o "${TMP_FILE}"

node - "${TMP_FILE}" <<'NODE'
const fs = require('node:fs')

const file = process.argv[2]
const data = JSON.parse(fs.readFileSync(file, 'utf8'))

if (
  (!data.models || Object.keys(data.models).length === 0) &&
  (!data.providers || Object.keys(data.providers).length === 0)
) {
  throw new Error('catalog has no models or providers')
}

fs.writeFileSync(file, JSON.stringify(data, null, 2) + '\n')
NODE

install -m 0644 "${TMP_FILE}" "${TARGET}"
echo "Updated ${TARGET} from ${SOURCE_URL}"
