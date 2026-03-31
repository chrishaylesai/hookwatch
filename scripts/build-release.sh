#!/usr/bin/env sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIST_DIR=${DIST_DIR:-"$ROOT_DIR/dist/release"}
FRONTEND_DIR="$ROOT_DIR/frontend"
APP_PKG=./cmd/hookwatch
TARGETS=${TARGETS:-"
linux amd64
linux arm64
darwin amd64
darwin arm64
windows amd64
windows arm64
"}
SKIP_NPM_CI=${SKIP_NPM_CI:-0}

require_cmd() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "missing required command: $1" >&2
		exit 1
	}
}

require_cmd go
require_cmd npm

mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/hookwatch-*

echo "==> Building frontend bundle"
if [ "$SKIP_NPM_CI" = "1" ]; then
	(cd "$FRONTEND_DIR" && npm run build)
else
	(cd "$FRONTEND_DIR" && npm ci && npm run build)
fi

echo "==> Building release binaries into $DIST_DIR"
printf '%s\n' "$TARGETS" | while read -r goos goarch; do
	[ -n "${goos:-}" ] || continue

	output="$DIST_DIR/hookwatch-${goos}-${goarch}"
	if [ "$goos" = "windows" ]; then
		output="${output}.exe"
	fi

	echo "-> ${goos}/${goarch}"
	(
		cd "$ROOT_DIR"
		CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
			go build -trimpath -ldflags="-s -w" -o "$output" "$APP_PKG"
	)
done

echo "==> Release binaries"
ls -lh "$DIST_DIR"
