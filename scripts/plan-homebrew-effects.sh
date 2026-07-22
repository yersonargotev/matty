#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --tap-dir DIR --formula FILE --repository OWNER/REPO --ref REF" >&2
  exit 2
}

tap_dir=
formula=
repository=
ref=
while (($#)); do
  case "$1" in
    --tap-dir) tap_dir="${2:-}"; shift 2 ;;
    --formula) formula="${2:-}"; shift 2 ;;
    --repository) repository="${2:-}"; shift 2 ;;
    --ref) ref="${2:-}"; shift 2 ;;
    *) usage ;;
  esac
done
[[ -n "$tap_dir" && -n "$formula" && -n "$repository" && -n "$ref" ]] || usage
[[ -d "$tap_dir/.git" ]] || { echo "tap observation is not a Git checkout" >&2; exit 1; }
[[ -f "$formula" && ! -L "$formula" ]] || { echo "candidate formula is not a regular non-symlink" >&2; exit 1; }
[[ -z "$(git -C "$tap_dir" status --porcelain --untracked-files=all)" ]] || {
  echo "tap observation is not clean" >&2; exit 1;
}

observed_commit="$(git -C "$tap_dir" rev-parse --verify HEAD)"
[[ "$observed_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "tap HEAD is not one commit" >&2; exit 1; }
expected_sha="$(sha256sum "$formula" | awk '{print $1}')"
packy_path="$tap_dir/Formula/packy.rb"
legacy_path="$tap_dir/Formula/matty.rb"

write_formula=true
if [[ -e "$packy_path" || -L "$packy_path" ]]; then
  [[ -f "$packy_path" && ! -L "$packy_path" ]] || {
    echo "observed Formula/packy.rb is not a regular non-symlink" >&2; exit 1;
  }
  [[ "$(sha256sum "$packy_path" | awk '{print $1}')" == "$expected_sha" ]] && write_formula=false
fi

delete_legacy=false
if [[ -e "$legacy_path" || -L "$legacy_path" ]]; then
  [[ -f "$legacy_path" && ! -L "$legacy_path" ]] || {
    echo "observed Formula/matty.rb is not a regular non-symlink" >&2; exit 1;
  }
  delete_legacy=true
fi

action=no-op
if [[ "$write_formula" == true || "$delete_legacy" == true ]]; then action=commit-and-push; fi
jq -cnS --arg action "$action" --arg repository "$repository" --arg ref "$ref" \
  --arg observed_commit "$observed_commit" --arg sha256 "$expected_sha" \
  --argjson write_formula "$write_formula" --argjson delete_legacy "$delete_legacy" '
  {action:$action,repository:$repository,ref:$ref,observed_commit:$observed_commit,
   formula:{path:"Formula/packy.rb",sha256:$sha256,write:$write_formula},
   delete_paths:(if $delete_legacy then ["Formula/matty.rb"] else [] end)}'
