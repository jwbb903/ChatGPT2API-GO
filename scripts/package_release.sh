#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

TARGET_OS="${TARGET_OS:-linux}"
TARGET_ARCH="${TARGET_ARCH:-amd64}"
APP_NAME="chatgpt2api-go"
PACKAGE_NAME="${APP_NAME}-${TARGET_OS}-${TARGET_ARCH}"
BIN_PATH="bin/${PACKAGE_NAME}"
RELEASE_DIR="release/${PACKAGE_NAME}"
TARBALL="release/${PACKAGE_NAME}.tar.gz"
BUILD_WEB=0
SKIP_TESTS=0

usage() {
  cat <<'EOF'
Usage: scripts/package_release.sh [options]

Options:
  --web         Rebuild frontend before packaging.
  --skip-tests  Skip go test ./....
  -h, --help    Show this help.

Environment:
  TARGET_OS     Target OS. Default: linux.
  TARGET_ARCH   Target architecture. Default: amd64.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --web)
      BUILD_WEB=1
      shift
      ;;
    --skip-tests)
      SKIP_TESTS=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

require_path() {
  local path="$1"
  local label="$2"
  if [[ ! -e "$path" ]]; then
    echo "Missing ${label}: ${path}" >&2
    exit 1
  fi
}

if [[ "$SKIP_TESTS" -eq 0 ]]; then
  echo "==> Running Go tests"
  go test ./...
fi

if [[ "$BUILD_WEB" -eq 1 ]]; then
  echo "==> Building frontend"
  make web
fi

require_path "web_dist" "frontend build output"
require_path "data/bin/curl-impersonate" "curl-impersonate directory"
require_path "start.sh" "start script"
require_path "README.md" "README"
require_path "GO_MIGRATION.md" "GO_MIGRATION.md"

mkdir -p bin release

echo "==> Building ${TARGET_OS}/${TARGET_ARCH} binary"
GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 \
  go build -trimpath -ldflags='-s -w' -o "$BIN_PATH" ./cmd/server

rm -rf "$RELEASE_DIR"
mkdir -p "$RELEASE_DIR/bin" "$RELEASE_DIR/data/bin"

cp "$BIN_PATH" "$RELEASE_DIR/bin/${APP_NAME}"
cp "$BIN_PATH" "$RELEASE_DIR/${APP_NAME}"
cp -R web_dist "$RELEASE_DIR/web_dist"
cp -R data/bin/curl-impersonate "$RELEASE_DIR/data/bin/curl-impersonate"
cp start.sh "$RELEASE_DIR/start.sh"
cp README.md GO_MIGRATION.md "$RELEASE_DIR/"

cat > "$RELEASE_DIR/config.example.json" <<'JSON'
{
  "server": {
    "addr": ":3000",
    "admin_key": "change-me"
  }
}
JSON

cat > "$RELEASE_DIR/README_RELEASE.md" <<'EOF'
# ChatGPT2API Go Linux amd64 Release

## Quick start

```bash
tar -xzf chatgpt2api-go-linux-amd64.tar.gz
cd chatgpt2api-go-linux-amd64
cp config.example.json config.json
# Edit config.json and change auth-key to your admin key.
./start.sh --port 3000
```

The default upstream transport is pure Go `tls-client` mode.

## Use bundled curl-impersonate

This package includes Linux x86_64 glibc curl-impersonate:

```text
data/bin/curl-impersonate/curl_edge101
```

Start with curl-impersonate:

```bash
./start.sh --curl --port 3000
```

Equivalent command:

```bash
CHATGPT2API_UPSTREAM_TRANSPORT=curl-impersonate \
CHATGPT2API_CURL_IMPERSONATE_BIN="$PWD/data/bin/curl-impersonate/curl_edge101" \
./bin/chatgpt2api-go
```

## Data directory

Runtime JSON data, images, and logs are stored under `data/` in the current directory.

## Notes

The bundled curl-impersonate binary is `x86_64-linux-gnu` and requires a normal Linux x86_64 glibc environment.
Alpine/musl, Android, and Termux cannot run this bundled binary directly.
EOF

chmod +x "$RELEASE_DIR/start.sh" "$RELEASE_DIR/${APP_NAME}" "$RELEASE_DIR/bin/${APP_NAME}"

rm -f "$TARBALL"
echo "==> Creating tarball"
(
  cd release
  tar -czf "${PACKAGE_NAME}.tar.gz" "$PACKAGE_NAME"
)

SHA256="$(sha256sum "$TARBALL" | awk '{print $1}')"
BIN_SIZE="$(ls -lh "$BIN_PATH" | awk '{print $5}')"
TARBALL_SIZE="$(ls -lh "$TARBALL" | awk '{print $5}')"
FILE_COUNT="$(tar -tzf "$TARBALL" | wc -l | tr -d ' ')"

cat <<EOF
==> Package complete
Binary:   ${BIN_PATH} (${BIN_SIZE})
Tarball:  ${TARBALL} (${TARBALL_SIZE})
SHA256:   ${SHA256}
Files:    ${FILE_COUNT}
EOF
