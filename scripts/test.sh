#!/usr/bin/env bash
# GLog 测试脚本
set -euo pipefail

echo "==> gofmt check"
UNFORMATTED="$(gofmt -l . | grep -v '^frontend/' || true)"
if [ -n "${UNFORMATTED}" ]; then
  echo "以下文件未格式化："
  echo "${UNFORMATTED}"
  exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./... -count=1

echo "==> frontend build (typecheck + vite)"
(cd frontend && npm install && npm run build)

echo "==> done"
