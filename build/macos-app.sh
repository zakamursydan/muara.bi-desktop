#!/bin/sh
# Assembles muara-bi.app for this machine. Three things have to end up inside
# one folder: the binary built with the pinned deployment target, the embedded
# UI, and the DuckDB connector bundle — Finder gives an app no shell
# environment, so the bundle has to live where the code can find it by itself.
#
# Usage: sh build/macos-app.sh [folder-bundle-extension]
set -eu

APP_NAME=muara-bi
BUNDLE_ID=bi.muara.desktop
SHORT_VERSION=0.0.1
BUILD_VERSION=1
MIN_MACOS=14.0

cd "$(dirname "$0")/.."
APP="build/bin/$APP_NAME.app"

if [ ! -d frontend/dist ]; then
	echo "frontend/dist is missing — run: (cd frontend && npm run build)" >&2
	exit 1
fi

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

# The pin is not cosmetic: Wails' own cgo directive otherwise writes minos 11.0
# while the DuckDB archive was compiled for 14.0 (docs/engine-map.md §13).
CGO_ENABLED=1 CGO_LDFLAGS="-mmacosx-version-min=$MIN_MACOS" \
	go build -trimpath -ldflags="-w -s" -o "$APP/Contents/MacOS/$APP_NAME" .

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key><string>$APP_NAME</string>
	<key>CFBundleDisplayName</key><string>muara.bi</string>
	<key>CFBundleExecutable</key><string>$APP_NAME</string>
	<key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>$SHORT_VERSION</string>
	<key>CFBundleVersion</key><string>$BUILD_VERSION</string>
	<key>LSMinimumSystemVersion</key><string>$MIN_MACOS</string>
	<key>NSHighResolutionCapable</key><true/>
	<key>NSPrincipalClass</key><string>NSApplication</string>
</dict>
</plist>
PLIST

# An empty extensions folder would be worse than none: the resolver would point
# DuckDB at it, the engine would then refuse to INSTALL, and the app would report
# "extension unavailable" for a bundle that was simply never passed.
if [ $# -ge 1 ] && [ -n "${1:-}" ]; then
	if [ ! -d "$1" ]; then
		echo "extension bundle '$1' does not exist" >&2
		exit 1
	fi
	mkdir -p "$APP/Contents/Resources/duckdb/extensions"
	cp -R "$1/." "$APP/Contents/Resources/duckdb/extensions/"
fi

# Ad-hoc signature: enough for a locally built app to launch (arm64 refuses
# unsigned binaries), not enough to pass Gatekeeper after a download. Real
# signing + notarization is M6.
codesign --force --sign - "$APP" >/dev/null 2>&1

minos=$(otool -l "$APP/Contents/MacOS/$APP_NAME" | awk '/LC_BUILD_VERSION/{found=1} found && /minos/{print $2; exit}')
[ "$minos" = "$MIN_MACOS" ] || { echo "linked minos is $minos, want $MIN_MACOS" >&2; exit 1; }
plutil -lint "$APP/Contents/Info.plist" >/dev/null

ext_count=0
if [ -d "$APP/Contents/Resources/duckdb/extensions" ]; then
	ext_count=$(find "$APP/Contents/Resources/duckdb/extensions" -name '*.duckdb_extension' | wc -l | tr -d ' ')
fi

echo "built $APP"
echo "  minos=$minos  extensions=$ext_count files  signature=$(codesign -dv "$APP" 2>&1 | awk -F= '/^Signature=/{print $2}')"
