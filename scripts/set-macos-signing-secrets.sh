#!/usr/bin/env bash
# Upload Apple Developer ID + App Store Connect API credentials as GitHub
# Actions secrets for ks1686/genv. See RELEASING.md for creating the files.
#
# Usage:
#   ./scripts/set-macos-signing-secrets.sh \
#     --p12 /path/to/Certificates.p12 \
#     --p12-password '…' \
#     --p8 /path/to/AuthKey_XXXXXXXXXX.p8 \
#     --key-id XXXXXXXXXX \
#     --issuer-id 00000000-0000-0000-0000-000000000000
#
# Optional:
#   --repo owner/name   (default: ks1686/genv)
set -euo pipefail

REPO="ks1686/genv"
P12_PATH=""
P12_PASSWORD=""
P8_PATH=""
KEY_ID=""
ISSUER_ID=""

usage() {
  sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --p12) P12_PATH="$2"; shift 2 ;;
    --p12-password) P12_PASSWORD="$2"; shift 2 ;;
    --p8) P8_PATH="$2"; shift 2 ;;
    --key-id) KEY_ID="$2"; shift 2 ;;
    --issuer-id) ISSUER_ID="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    -h|--help) usage ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      ;;
  esac
done

if [[ -z "$P12_PATH" || -z "$P12_PASSWORD" || -z "$P8_PATH" || -z "$KEY_ID" || -z "$ISSUER_ID" ]]; then
  echo "missing required arguments" >&2
  usage
fi
if [[ ! -f "$P12_PATH" ]]; then
  echo "p12 not found: $P12_PATH" >&2
  exit 1
fi
if [[ ! -f "$P8_PATH" ]]; then
  echo "p8 not found: $P8_PATH" >&2
  exit 1
fi
if ! command -v gh >/dev/null 2>&1; then
  echo "gh is required" >&2
  exit 1
fi
if ! command -v base64 >/dev/null 2>&1; then
  echo "base64 is required" >&2
  exit 1
fi

# macOS base64 uses -i; GNU base64 uses -w0.
b64_file() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    base64 -i "$1" | tr -d '\n'
  else
    base64 -w0 <"$1"
  fi
}

echo "Setting MACOS_* secrets on $REPO …"
gh secret set MACOS_SIGN_P12 --repo "$REPO" --body "$(b64_file "$P12_PATH")"
gh secret set MACOS_SIGN_PASSWORD --repo "$REPO" --body "$P12_PASSWORD"
gh secret set MACOS_NOTARY_KEY --repo "$REPO" --body "$(b64_file "$P8_PATH")"
gh secret set MACOS_NOTARY_KEY_ID --repo "$REPO" --body "$KEY_ID"
gh secret set MACOS_NOTARY_ISSUER_ID --repo "$REPO" --body "$ISSUER_ID"

echo "Done. Verify with: gh secret list --repo $REPO"
