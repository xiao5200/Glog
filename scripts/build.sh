#!/usr/bin/env bash
# GLog 跨平台构建脚本（需在项目根目录执行）
set -euo pipefail

VERSION="${VERSION:-1.0.0}"
OUTPUT_DIR="${OUTPUT_DIR:-./dist}"
export PATH="$PATH:$(go env GOPATH)/bin"

echo "==> GLog v${VERSION} build"

rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

echo "==> Install frontend dependencies"
(cd frontend && npm install)

echo "==> Build Windows amd64"
wails build -platform windows/amd64 -clean
cp build/bin/GLog.exe "${OUTPUT_DIR}/GLog-${VERSION}-windows-amd64.exe"

if command -v wails >/dev/null 2>&1; then
  if [ "$(uname -s)" = "Linux" ]; then
    echo "==> Build Linux amd64"
    wails build -platform linux/amd64 -clean
    cp build/bin/GLog "${OUTPUT_DIR}/GLog-${VERSION}-linux-amd64"
  fi
fi

(cd "${OUTPUT_DIR}" && sha256sum * > checksums.txt 2>/dev/null || true)
echo "==> Done: ${OUTPUT_DIR}"
