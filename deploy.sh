#!/usr/bin/env bash
# Деплой на нетбук: тесты → сборка → копирование → перезапуск → проверка.
#
#   ./deploy.sh                       # хост netbook из ~/.ssh/config
#   DEPLOY_HOST=user@192.168.1.5 ./deploy.sh
#   FORCE=1 ./deploy.sh               # залить незакоммиченный код
set -euo pipefail

host="${DEPLOY_HOST:-netbook}"

cd "$(dirname "$0")"

if [[ -n "$(git status --porcelain -- cmd internal web go.mod go.sum)" && "${FORCE:-}" != 1 ]]; then
    echo "Есть незакоммиченные изменения в коде:" >&2
    git status --short -- cmd internal web go.mod go.sum >&2
    echo "Закоммить их или запусти с FORCE=1." >&2
    exit 1
fi

commit="$(git rev-parse --short HEAD)"
echo "==> Коммит $commit, хост $host"

echo "==> go vet и go test"
go vet ./...
go test ./...

binary="$(mktemp)"
trap 'rm -f "$binary"' EXIT

echo "==> Сборка linux/amd64"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$binary" ./cmd/filevault

echo "==> Копирование на $host"
scp -q "$binary" "$host:filevault.new"

echo "==> Установка и перезапуск"
ssh -t "$host" '
    set -e
    sudo install -m 755 ~/filevault.new /opt/filevault/filevault
    rm ~/filevault.new
    sudo systemctl restart filevault
    sleep 2
    systemctl is-active filevault
    wget -qO /dev/null localhost:8080/api/dirs && echo "API отвечает"
'

echo "==> Готово: на $host работает $commit"
