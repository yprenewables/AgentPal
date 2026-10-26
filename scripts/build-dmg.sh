#!/bin/bash
# Usage: bash scripts/build-dmg.sh [AgentPal.app] [output-directory]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="${1:-$ROOT/build/bin/AgentPal.app}"
OUTPUT="${2:-$ROOT/build/release}"
VERSION=$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")
ARCH=$(lipo -archs "$APP/Contents/MacOS/AgentPal")
case "$ARCH" in
  arm64|x86_64) ;;
  "x86_64 arm64"|"arm64 x86_64") ARCH=universal ;;
  *) printf 'Unsupported architecture: %s\n' "$ARCH" >&2; exit 1 ;;
esac

codesign --verify --deep --strict "$APP"
mkdir -p "$OUTPUT"
OUTPUT="$(cd "$OUTPUT" && pwd)"
DMG="$OUTPUT/AgentPal-$VERSION-macos-$ARCH.dmg"
if [[ -e "$DMG" ]]; then
  printf 'Output already exists: %s\n' "$DMG" >&2
  exit 1
fi

WORK=$(mktemp -d "${TMPDIR:-/tmp/}agentpal-dmg.XXXXXX")
MOUNT="$WORK/mount"
cleanup() {
  if mount | /usr/bin/grep -Fq " on $MOUNT ("; then
    hdiutil detach "$MOUNT" -quiet || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT
mkdir -p "$WORK/content" "$MOUNT"
ditto "$APP" "$WORK/content/AgentPal.app"
ln -s /Applications "$WORK/content/Applications"
cp "$ROOT/build/dmg/安装说明.txt" "$WORK/content/安装说明.txt"
hdiutil create -volname "AgentPal $VERSION" -srcfolder "$WORK/content" \
  -fs HFS+ -format UDRW "$WORK/editable.dmg" -quiet
hdiutil attach "$WORK/editable.dmg" -mountpoint "$MOUNT" -nobrowse -quiet

# Finder stores the window layout in .DS_Store inside the image.
osascript - "$MOUNT" <<'APPLESCRIPT'
on run argv
  set volumeFolder to POSIX file (item 1 of argv) as alias
  tell application "Finder"
    open volumeFolder
    set installerWindow to window of volumeFolder
    set current view of installerWindow to icon view
    set toolbar visible of installerWindow to false
    set statusbar visible of installerWindow to false
    set bounds of installerWindow to {200, 160, 840, 580}
    set viewOptions to icon view options of installerWindow
    set arrangement of viewOptions to not arranged
    set icon size of viewOptions to 96
    set text size of viewOptions to 14
    set position of item "AgentPal.app" of volumeFolder to {150, 130}
    set position of item "Applications" of volumeFolder to {460, 130}
    set position of item "安装说明.txt" of volumeFolder to {300, 295}
    close installerWindow
    open volumeFolder
    update volumeFolder without registering applications
    delay 2
    close window of volumeFolder
  end tell
end run
APPLESCRIPT

sync
hdiutil detach "$MOUNT" -quiet
hdiutil convert "$WORK/editable.dmg" -format UDZO -imagekey zlib-level=9 -o "$DMG" -quiet
hdiutil verify "$DMG"
shasum -a 256 "$DMG"
