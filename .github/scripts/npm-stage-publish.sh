#!/usr/bin/env bash
# Stages one npm package version, retrying the transient Sigstore failures.
#
# `npm stage publish --provenance` signs the provenance statement and records
# it in the Rekor transparency log before it talks to the npm registry. When
# Rekor stores the entry but the response is lost, the client's retry gets a
# 409 "an equivalent entry already exists" and npm aborts with
# TLOG_CREATE_ENTRY_ERROR even though nothing was staged. A fresh attempt signs
# with a new ephemeral key, so it records a new entry and succeeds.
#
# Usage: npm-stage-publish.sh <package-dir-or-tarball> [npm stage publish options...]
#
# NPM_STAGE_ATTEMPTS (default 3) and NPM_STAGE_RETRY_DELAY seconds (default 15)
# tune the retry. Errors that are not transient fail on the first attempt.
# A network error during the registry POST itself may leave the version
# staged; the retry then meets npm's "already staged" refusal and fails,
# which is the right outcome: approve or reject it before re-running.
set -euo pipefail

attempts=${NPM_STAGE_ATTEMPTS:-3}
delay=${NPM_STAGE_RETRY_DELAY:-15}
case "$attempts$delay" in *[!0-9]*)
  echo "::error::NPM_STAGE_ATTEMPTS and NPM_STAGE_RETRY_DELAY must be whole numbers"
  exit 2
  ;;
esac
transient='TLOG_CREATE_ENTRY_ERROR|TLOG_FETCH_ENTRY_ERROR|ECONNRESET|ETIMEDOUT|EAI_AGAIN|npm error code E50[0-9]'

log=$(mktemp)
trap 'rm -f "$log"' EXIT

attempt=1
while :; do
  if npm stage publish "$@" 2>&1 | tee "$log"; then
    exit 0
  fi
  if ! grep -Eq "$transient" "$log"; then
    exit 1
  fi
  if [ "$attempt" -ge "$attempts" ]; then
    echo "::error::npm stage publish failed with a transient error $attempts times"
    exit 1
  fi
  echo "::warning::npm stage publish failed with a transient error (attempt $attempt of $attempts); retrying in ${delay}s"
  sleep "$delay"
  attempt=$((attempt + 1))
done
