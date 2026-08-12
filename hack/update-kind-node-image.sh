#!/usr/bin/env bash

set -euo pipefail

# Updates KIND_NODE_IMAGE in .github/workflows/ci.yaml to a node image that is
# actually shipped for the KIND_VERSION pinned there. The kind -> node image
# mapping only exists in the kind release notes, so this script fetches the
# notes for the pinned kind version and parses the images listed there.
#
# The script is idempotent: it leaves the image untouched when it is already
# one of the images listed for the current KIND_VERSION (including when the
# user intentionally pinned an older, still-supported image), and otherwise
# rewrites it to the release's default image.
#
# It runs as a Renovate postUpgradeTask so KIND_NODE_IMAGE is bumped in the
# same PR that bumps KIND_VERSION. It can also be run manually:
#   ./hack/update-kind-node-image.sh [path/to/ci.yaml]

REPO_ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )"
CI_FILE="${1:-${REPO_ROOT_DIR}/.github/workflows/ci.yaml}"

python3 - "${CI_FILE}" <<'PY'
import json
import re
import sys
import urllib.request

ci_file = sys.argv[1]

with open(ci_file) as f:
    content = f.read()


def var(name):
    m = re.search(rf'^[ \t]*{name}:\s*(.+?)\s*$', content, re.MULTILINE)
    if not m:
        sys.exit(f'error: could not find {name} in {ci_file}')
    return m.group(1).strip("\"'")


kind_version = var('KIND_VERSION')
current_image = var('KIND_NODE_IMAGE')

url = f'https://api.github.com/repos/kubernetes-sigs/kind/releases/tags/{kind_version}'
req = urllib.request.Request(url, headers={'User-Agent': 'ollama-operator-renovate'})
try:
    with urllib.request.urlopen(req, timeout=30) as resp:
        release = json.load(resp)
except Exception as e:  # noqa: BLE001
    sys.exit(f'error: failed to fetch kind {kind_version} release notes from {url}: {e}')

# The release notes list the images pre-built for this release as:
#   - v1.33.1: `kindest/node:v1.33.1@sha256:8d866994839c...`
# The first occurrence is the release's default node image.
images = re.findall(r'kindest/node:v\d+\.\d+\.\d+@sha256:[a-f0-9]{64}', release.get('body', ''))
if not images:
    sys.exit(f'error: no node images found in kind {kind_version} release notes')

default_image = images[0]

if current_image == default_image:
    print(f'KIND_NODE_IMAGE is already the default for kind {kind_version}: {current_image}')
    sys.exit(0)

if current_image in images:
    print(f'KIND_NODE_IMAGE {current_image} is still supported by kind {kind_version}, leaving unchanged')
    sys.exit(0)

new_content = re.sub(
    r'^(?P<indent>[ \t]*)KIND_NODE_IMAGE:\s*.*$',
    lambda m: f"{m.group('indent')}KIND_NODE_IMAGE: {default_image}",
    content,
    count=1,
    flags=re.MULTILINE,
)
with open(ci_file, 'w') as f:
    f.write(new_content)

print(f'Updated KIND_NODE_IMAGE: {current_image} -> {default_image} (default for kind {kind_version})')
PY
