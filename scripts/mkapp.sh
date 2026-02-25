#!/usr/bin/env bash
# mkapp.sh — Assemble a macOS .app bundle from a pre-built binary.
#
# Usage: mkapp.sh <output.app> <binary> <icon.png> <version>
#
# The icon.png is converted to icon.icns via sips (ships with macOS).
# If sips is unavailable, the .app is created without an icon.

set -euo pipefail

APP_PATH="$1"
BINARY="$2"
ICON_PNG="$3"
VERSION="${4:-dev}"
BUNDLE_ID="com.grotto.client"

rm -rf "$APP_PATH"
mkdir -p "$APP_PATH/Contents/MacOS"
mkdir -p "$APP_PATH/Contents/Resources"

# Copy binary
cp "$BINARY" "$APP_PATH/Contents/MacOS/grotto"
chmod +x "$APP_PATH/Contents/MacOS/grotto"

# Convert PNG to icns using sips + iconutil (both ship with macOS)
if command -v iconutil &>/dev/null && command -v sips &>/dev/null; then
  ICONSET=$(mktemp -d)/Grotto.iconset
  mkdir -p "$ICONSET"
  for size in 16 32 64 128 256 512; do
    sips -z $size $size "$ICON_PNG" --out "$ICONSET/icon_${size}x${size}.png" &>/dev/null
    double=$((size * 2))
    if [ $double -le 1024 ]; then
      sips -z $double $double "$ICON_PNG" --out "$ICONSET/icon_${size}x${size}@2x.png" &>/dev/null
    fi
  done
  iconutil -c icns -o "$APP_PATH/Contents/Resources/icon.icns" "$ICONSET"
  rm -rf "$(dirname "$ICONSET")"
else
  echo "warning: iconutil/sips not found, .app will have no icon" >&2
fi

# Write Info.plist
cat > "$APP_PATH/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Grotto</string>
	<key>CFBundleExecutable</key>
	<string>grotto</string>
	<key>CFBundleIdentifier</key>
	<string>${BUNDLE_ID}</string>
	<key>CFBundleIconFile</key>
	<string>icon.icns</string>
	<key>CFBundleShortVersionString</key>
	<string>${VERSION}</string>
	<key>CFBundleSupportedPlatforms</key>
	<array>
		<string>MacOSX</string>
	</array>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSSupportsAutomaticGraphicsSwitching</key>
	<true/>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
</dict>
</plist>
PLIST

echo "Created $APP_PATH (v${VERSION})"
