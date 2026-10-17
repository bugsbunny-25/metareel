#!/usr/bin/env bash
# Deletes every main-branch image of a GHCR package except the one just
# published (KEEP_DIGEST), so main keeps a single image.
#
# A main image is a version whose tags are all `main` / `sha-<commit>`; release
# images carry a v* tag and are never selected. Only versions created before
# the kept one are deleted, so a slower older run can't remove a newer image.
# Each multi-arch image is an index plus untagged per-platform and attestation
# manifests; those children are deleted with it unless a kept image uses them.
#
# Env: OWNER, PACKAGE, KEEP_DIGEST, GH_TOKEN (packages:write). DRY_RUN=true
# lists what would go without deleting. Needs `gh`, `jq` and `docker` logged
# in to ghcr.io (for reading manifests).
set -euo pipefail

: "${OWNER:?}" "${PACKAGE:?}" "${KEEP_DIGEST:?}"
DRY_RUN="${DRY_RUN:-false}"
IMAGE="ghcr.io/$(tr '[:upper:]' '[:lower:]' <<<"${OWNER}")/${PACKAGE}"

# Shared jq: `$stale` is the main images to delete — all tags `main` /
# `sha-<commit>`, not the kept digest, created before it.
JQ_STALE='
  def tags: .metadata.container.tags;
  def is_main: (tags | length) > 0
    and all(tags[]; test("^(main|sha-[0-9a-f]{7,40})$"));
  (map(select(.name == $keep)) | first) as $cur
  | if $cur == null then error("kept digest \($keep) is not in the package") else . end
  | (map(select(is_main and .name != $keep and .created_at < $cur.created_at))) as $stale'

# Prints `id digest` for each main image to delete. Input: the package's
# versions as one JSON array.
select_stale_main() {
  jq -r --arg keep "$1" "${JQ_STALE}"' | $stale[] | "\(.id) \(.name)"'
}

# Prints the digest of every tagged version that stays.
select_kept_tagged() {
  jq -r --arg keep "$1" "${JQ_STALE}"'
    | ($stale | map(.name)) as $gone
    | .[] | select((tags | length) > 0 and (.name as $n | $gone | index($n) | not))
    | .name'
}

# Digests an image index points at (platform images, attestations).
children_of() {
  docker buildx imagetools inspect --raw "${IMAGE}@$1" | jq -r '.manifests[]?.digest'
}

delete_version() {
  local base="$1" id="$2" digest="$3" what="$4"
  if [[ "${DRY_RUN}" == "true" ]]; then
    echo "would delete ${what} ${digest} (version ${id})"
  else
    gh api -X DELETE "${base}/versions/${id}" >/dev/null
    echo "deleted ${what} ${digest} (version ${id})"
  fi
}

main() {
  local base versions stale protected deleted=0 id digest children child child_id
  if [[ "$(gh api "/users/${OWNER}" --jq .type)" == "Organization" ]]; then
    base="/orgs/${OWNER}/packages/container/${PACKAGE}"
  else
    base="/users/${OWNER}/packages/container/${PACKAGE}"
  fi

  versions="$(gh api --paginate "${base}/versions?per_page=100" | jq -s 'add')"
  stale="$(select_stale_main "${KEEP_DIGEST}" <<<"${versions}")"
  if [[ -z "${stale}" ]]; then
    echo "No older main images to delete."
    return
  fi

  # Every image that stays, and everything it points at, must survive. A
  # manifest that can't be read fails the run before anything is deleted.
  protected="$(select_kept_tagged "${KEEP_DIGEST}" <<<"${versions}" | while read -r digest; do
    echo "${digest}"
    children_of "${digest}" || exit 1
  done)"

  while read -r id digest; do
    # Read the children before the index is gone.
    children="$(children_of "${digest}" || true)"
    delete_version "${base}" "${id}" "${digest}" "image"
    deleted=$((deleted + 1))
    while read -r child; do
      [[ -z "${child}" ]] && continue
      grep -qxF "${child}" <<<"${protected}" && continue
      child_id="$(jq -r --arg d "${child}" '.[] | select(.name == $d) | .id' <<<"${versions}")"
      [[ -z "${child_id}" ]] && continue
      delete_version "${base}" "${child_id}" "${child}" "  manifest"
    done <<<"${children}"
  done <<<"${stale}"

  echo "Kept ${KEEP_DIGEST}; removed ${deleted} older main image(s)."
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    echo "Kept main image \`${KEEP_DIGEST}\`, removed ${deleted} older one(s)." >> "${GITHUB_STEP_SUMMARY}"
  fi
}

main "$@"
