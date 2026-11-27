#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="$repo_root/dist"
goroot="$(go env GOROOT)"

mkdir -p "$dist_dir"
cp -R "$repo_root/web/." "$dist_dir/"
sed 's|<!-- PAGES_WASM_RUNTIME -->|<script src="wasm_exec.js" defer></script>|' \
  "$repo_root/web/index.html" > "$dist_dir/index.html"
(cd "$repo_root" && GOOS=js GOARCH=wasm go build -o "$dist_dir/roofline.wasm" .)
cp "$goroot/lib/wasm/wasm_exec.js" "$dist_dir/wasm_exec.js"
touch "$dist_dir/.nojekyll"

echo "GitHub Pages site built in $dist_dir"
