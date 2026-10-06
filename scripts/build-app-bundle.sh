#!/bin/zsh

set -euo pipefail

repository_root=${0:A:h:h}
bundle="$repository_root/dist/Personal Search.app"
contents="$bundle/Contents"
resources="$contents/Resources"
signing_identity=${CODESIGN_IDENTITY:--}
worker_build="$repository_root/tmp/pyinstaller"
semantic_model_cache="$repository_root/tmp/embedding-model"

if [[ ! -x "$repository_root/extractor/.venv/bin/pyinstaller" ]]; then
    print -u2 "Run 'make setup-extractor' before building the app bundle"
    exit 1
fi

rm -rf "$bundle"
rm -rf "$worker_build"
mkdir -p "$contents/MacOS" "$resources/Core" "$resources/Extractor" "$resources/Models" "$worker_build/spec" "$worker_build/work" "$worker_build/dist"

"$repository_root/scripts/prepare-semantic-model.sh" >/dev/null
cp -R "$semantic_model_cache" "$resources/Models/FastEmbed"

swift build --package-path "$repository_root/app" -c release
go build -C "$repository_root/core" -tags sqlite_fts5 -o "$resources/Core/personal-search-core" ./cmd/personal-search-core

swift_binary="$repository_root/app/.build/arm64-apple-macosx/release/PersonalSearch"
cp "$swift_binary" "$contents/MacOS/PersonalSearch"
"$repository_root/extractor/.venv/bin/pyinstaller" \
    --clean \
    --noconfirm \
    --onefile \
    --name personal-search-extractor \
    --paths "$repository_root/extractor/src" \
    --specpath "$worker_build/spec" \
    --workpath "$worker_build/work" \
    --distpath "$worker_build/dist" \
    "$repository_root/extractor/src/personal_search_extractor/__main__.py"
cp "$worker_build/dist/personal-search-extractor" "$resources/Extractor/personal-search-extractor"

plist="$contents/Info.plist"
plutil -create xml1 "$plist"
plutil -insert CFBundleDevelopmentRegion -string en "$plist"
plutil -insert CFBundleExecutable -string PersonalSearch "$plist"
plutil -insert CFBundleIdentifier -string dev.fiqi.personal-search "$plist"
plutil -insert CFBundleInfoDictionaryVersion -string 6.0 "$plist"
plutil -insert CFBundleName -string "Personal Search" "$plist"
plutil -insert CFBundleDisplayName -string "Personal Search" "$plist"
plutil -insert CFBundlePackageType -string APPL "$plist"
plutil -insert CFBundleShortVersionString -string 1.0.0 "$plist"
plutil -insert CFBundleVersion -string 1 "$plist"
plutil -insert LSMinimumSystemVersion -string 14.0 "$plist"
plutil -insert NSHighResolutionCapable -bool true "$plist"
plutil -insert NSPrincipalClass -string NSApplication "$plist"

chmod 755 "$contents/MacOS/PersonalSearch" "$resources/Core/personal-search-core" "$resources/Extractor/personal-search-extractor"

codesign --force --deep --options runtime --sign "$signing_identity" "$bundle"
codesign --verify --deep --strict --verbose=2 "$bundle"

print "$bundle"
