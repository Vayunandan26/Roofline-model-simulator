#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="$repo_root/dist"
goroot="$(go env GOROOT)"

rm -rf "$dist_dir"
mkdir -p "$dist_dir"

# Copy frontend assets
cp -R "$repo_root/web/." "$dist_dir/"

# Compile Go WASM directly into dist/
(cd "$repo_root" && GOOS=js GOARCH=wasm go build -o "$dist_dir/roofline.wasm" .)

# Copy wasm_exec.js (supports both Go 1.24+ and Go <= 1.23)
if [ -f "$goroot/lib/wasm/wasm_exec.js" ]; then
  cp "$goroot/lib/wasm/wasm_exec.js" "$dist_dir/wasm_exec.js"
elif [ -f "$goroot/misc/wasm/wasm_exec.js" ]; then
  cp "$goroot/misc/wasm/wasm_exec.js" "$dist_dir/wasm_exec.js"
else
  echo "Error: wasm_exec.js not found in $goroot" >&2
  exit 1
fi

touch "$dist_dir/.nojekyll"
echo "GitHub Pages site built in $dist_dir"