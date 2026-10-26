#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Create a network-independent Linux x86_64 OCR CLI tarball.
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: $0 <linux-amd64-binary> <output-dir> <release-version>" >&2
  exit 2
fi

binary="$1"
output_dir="$2"
release_version="$3"

if [[ ! -f "$binary" || ! -s "$binary" ]]; then
  echo "Missing or empty Linux AMD64 binary: $binary" >&2
  exit 1
fi
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

pkg="$stage/linux-x86-64-offline"
mkdir -p "$pkg"
install -m 0755 "$binary" "$pkg/ocr"

cat > "$pkg/install.sh" <<'INSTALL'
#!/usr/bin/env sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
target="${1:-${HOME}/.local/bin}"
mkdir -p "$target"
install -m 0755 "$root/ocr" "$target/ocr"
printf 'Installed ocr to %s/ocr\n' "$target"
printf 'Run: %s/ocr --version\n' "$target"
INSTALL
chmod 0755 "$pkg/install.sh"

cat > "$pkg/README.md" <<'DOC'
# OpenCodeReview SVN fork — Linux x86_64 offline package

Release: v@RELEASE_VERSION@
Platform: Linux x86_64 / AMD64
Source: https://github.com/smallfishworld/open-code-review-svn

This package contains the prebuilt `ocr` CLI binary. You do not need
Node.js, npm, Go, or external network access to install it.

## Verify contents

```sh
cd linux-x86-64-offline
sha256sum -c SHA256SUMS
```

## Run directly, without installation

```sh
./ocr --version
./ocr review --repo /path/to/svn-working-copy --preview
```

## Install for the current user (no root required)

```sh
./install.sh
~/.local/bin/ocr --version
```

Add `~/.local/bin` to PATH if necessary. For a system-wide install:

```sh
sudo ./install.sh /usr/local/bin
```

## SVN review examples

```sh
ocr review --repo /path/to/svn-working-copy --preview
ocr review --repo /path/to/svn-working-copy --commit 1234
ocr review --repo /path/to/svn-working-copy --from 1200 --to 1234
```

SVN >= 1.9 must be installed if you want SVN reviews. For AI-powered
reviews, configure a reachable model API (a LAN-hosted model works);
`--preview` does not make AI API requests. Installation itself is offline.

The binary comes from the corresponding GitHub Release artifact
`opencodereview-linux-amd64`.
DOC

sed -i "s/@RELEASE_VERSION@/${release_version}/g" "$pkg/README.md"

(
  cd "$pkg"
  sha256sum ocr install.sh README.md > SHA256SUMS
  sha256sum -c SHA256SUMS
)

archive="$output_dir/linux-x86-64-offline.tar.gz"
tar -C "$stage" -czf "$archive" linux-x86-64-offline
tar -tzf "$archive" >/dev/null

echo "Created: $archive"
sha256sum "$archive"
