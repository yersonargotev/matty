#!/usr/bin/env bash
set -euo pipefail

repo= ref= commit= workflow_sha= output=
while (($#)); do
  case "$1" in
    --repo) repo="${2:-}"; shift 2 ;;
    --ref) ref="${2:-}"; shift 2 ;;
    --commit) commit="${2:-}"; shift 2 ;;
    --workflow-sha) workflow_sha="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[[ -n "$repo" && -n "$ref" && -n "$commit" && -n "$workflow_sha" && -n "$output" ]] || {
  echo "usage: collect-governance-drift.sh --repo OWNER/REPO --ref REF --commit SHA --workflow-sha SHA --output FILE" >&2
  exit 2
}
command -v jq >/dev/null || { echo "jq is required" >&2; exit 2; }
GH_BIN="${GH_BIN:-gh}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
controls="$tmp/controls.jsonl"
: >"$controls"

# API responses are projected by gh/jq before they enter the collector. A failed
# request never contributes its response body to durable evidence.
control() {
  local id="$1" endpoint="$2" projection="$3" result="$tmp/result.json"
  if ! "$GH_BIN" api "$endpoint" --jq "$projection" >"$result" 2>/dev/null; then
    jq -cn --arg id "$id" '{id:$id,state:"collection-failure",detail:"read-only collection failed"}' >>"$controls"
  elif ! jq -e 'type=="object" and (.valid|type=="boolean") and has("actual")' "$result" >/dev/null 2>&1; then
    jq -cn --arg id "$id" '{id:$id,state:"unclassifiable",detail:"required API shape was not recognized"}' >>"$controls"
  elif [[ "$(jq -r '.valid' "$result")" != true ]]; then
    jq -cn --arg id "$id" '{id:$id,state:"unclassifiable",detail:"required API shape was not recognized"}' >>"$controls"
  else
    jq -cS --arg id "$id" '{id:$id,state:"observed",actual:.actual}' "$result" >>"$controls"
  fi
}

failure() { jq -cn --arg id "$1" '{id:$id,state:"collection-failure",detail:"read-only collection failed"}' >>"$controls"; }
unclassifiable() { jq -cn --arg id "$1" '{id:$id,state:"unclassifiable",detail:"required API shape was not recognized"}' >>"$controls"; }
observed() { jq -cS --arg id "$1" '{id:$id,state:"observed",actual:.}' "$2" >>"$controls"; }
project() {
  local endpoint="$1" projection="$2" destination="$3"
  "$GH_BIN" api "$endpoint" --jq "$projection" >"$destination" 2>/dev/null || return 1
  jq -e '. != null' "$destination" >/dev/null 2>&1 || return 2
}

control repository-settings "repos/$repo" 'if type=="object" and (.visibility|type)=="string" and (.default_branch|type)=="string" and (.archived|type)=="boolean" then {valid:true,actual:{visibility,default_branch,archived,allow_merge_commit,allow_squash_merge,allow_rebase_merge,allow_auto_merge,delete_branch_on_merge,web_commit_signoff_required}} else {valid:false,actual:null} end'

control actions-policy "repos/$repo/actions/permissions" 'if type=="object" and (.enabled|type)=="boolean" and (.allowed_actions|type)=="string" then {valid:true,actual:{enabled,allowed_actions,sha_pinning_required}} else {valid:false,actual:null} end'
control workflow-policy "repos/$repo/actions/permissions/workflow" 'if type=="object" and (.default_workflow_permissions|type)=="string" and (.can_approve_pull_request_reviews|type)=="boolean" then {valid:true,actual:{default_workflow_permissions,can_approve_pull_request_reviews}} else {valid:false,actual:null} end'
control main-protection "repos/$repo/branches/main/protection" 'if type=="object" and (.required_status_checks|type)=="object" and (.required_pull_request_reviews|type)=="object" then {valid:true,actual:{required_status_checks:{strict:.required_status_checks.strict,checks:(.required_status_checks.checks//[]|map({context,app_id})|sort_by(.context,.app_id))},required_pull_request_reviews:{dismiss_stale_reviews:.required_pull_request_reviews.dismiss_stale_reviews,require_code_owner_reviews:.required_pull_request_reviews.require_code_owner_reviews,required_approving_review_count:.required_pull_request_reviews.required_approving_review_count,require_last_push_approval:.required_pull_request_reviews.require_last_push_approval},enforce_admins:(.enforce_admins.enabled//false),required_conversation_resolution:(.required_conversation_resolution.enabled//false),restrictions:(.restrictions|if .==null then null else {users:(.users//[]|map(.login)|sort),teams:(.teams//[]|map(.slug)|sort),apps:(.apps//[]|map(.slug)|sort)} end),allow_force_pushes:(.allow_force_pushes.enabled//false),allow_deletions:(.allow_deletions.enabled//false)}} else {valid:false,actual:null} end'
ruleset_index="$tmp/ruleset-index.json"; ruleset_rc=0
project "repos/$repo/rulesets?includes_parents=true&per_page=100" 'if type=="array" and all(.[]; (.id|type)=="number" and (.name|type)=="string" and (.target|type)=="string" and (.enforcement|type)=="string") then [.[]|select(.target=="tag")|{id,name,target,enforcement}] else null end' "$ruleset_index" || ruleset_rc=$?
printf '[]\n' >"$tmp/tag-rules.json"
if ((ruleset_rc==0)); then
  while read -r ruleset_id; do
    detail="$tmp/ruleset-$ruleset_id.json"; detail_rc=0
    project "repos/$repo/rulesets/$ruleset_id" 'if type=="object" and (.name|type)=="string" and (.target|type)=="string" and (.enforcement|type)=="string" and (.rules|type)=="array" then {name,target,enforcement,conditions,bypass_actors:(.bypass_actors//[]|map({actor_type,bypass_mode})|sort_by(.actor_type,.bypass_mode)),rules:(.rules|map({type,parameters}|with_entries(select(.value!=null)))|sort_by(.type))} else null end' "$detail" || detail_rc=$?
    if ((detail_rc!=0)); then ruleset_rc=$detail_rc; break; fi
    jq --slurpfile detail "$detail" '.+[$detail[0]]|sort_by(.name)' "$tmp/tag-rules.json" >"$tmp/tag-rules.next" && mv "$tmp/tag-rules.next" "$tmp/tag-rules.json"
  done < <(jq -r '.[].id' "$ruleset_index")
fi
if ((ruleset_rc==1)); then failure tag-rules
elif ((ruleset_rc==2)); then unclassifiable tag-rules
else observed tag-rules "$tmp/tag-rules.json"
fi
env_base="$tmp/environments.json"
env_rc=0
project "repos/$repo/environments?per_page=100" 'if type=="object" and (.environments|type)=="array" then [.environments[]|{name,can_admins_bypass:(.can_admins_bypass//false),deployment_branch_policy:{protected_branches:(.deployment_branch_policy.protected_branches//false),custom_branch_policies:(.deployment_branch_policy.custom_branch_policies//false)},reviewers:[(.protection_rules//[])[]|select(.type=="required_reviewers")|.reviewers[]?|{login:(.reviewer.login//.reviewer.slug//.reviewer.name),type:(.reviewer.type//"unknown"),prevent_self_review:(.prevent_self_review//false)}]|sort_by(.login,.type),branch_policies:[]}]|sort_by(.name) else null end' "$env_base" || env_rc=$?
if ((env_rc==1)); then
  failure protected-environments
elif ((env_rc==2)); then
  unclassifiable protected-environments
else
  env_failed=false
  while IFS=$'\t' read -r env_name env_path; do
    policies="$tmp/policies-$(printf '%s' "$env_name" | shasum | cut -d' ' -f1).json"
    if ! project "repos/$repo/environments/$env_path/deployment-branch-policies?per_page=100" 'if type=="object" and (.branch_policies|type)=="array" then [.branch_policies[]|{name,type}]|sort_by(.name,.type) else error("shape") end' "$policies"; then env_failed=true; break; fi
    jq --arg name "$env_name" --slurpfile policies "$policies" 'map(if .name==$name then .branch_policies=$policies[0] else . end)' "$env_base" >"$env_base.next"
    mv "$env_base.next" "$env_base"
  done < <(jq -r '.[]|[.name,(.name|@uri)]|@tsv' "$env_base")
  if [[ "$env_failed" == true ]]; then failure protected-environments; else observed protected-environments "$env_base"; fi
fi

# Credential values are never readable; retain only names and timestamps.
cred_repo="$tmp/credential-repository.json"
cred_rc=0
project "repos/$repo/actions/secrets?per_page=100" 'if type=="object" and (.total_count|type)=="number" and (.secrets|type)=="array" then {total_count,secrets:[.secrets[]|{name,created_at,updated_at}]|sort_by(.name)} else null end' "$cred_repo" || cred_rc=$?
if ((cred_rc==1)); then
  failure credential-metadata
elif ((cred_rc==2)); then
  unclassifiable credential-metadata
else
  jq -nS --slurpfile repository "$cred_repo" '{repository_actions:$repository[0],environments:[]}' >"$tmp/credentials.json"
  cred_failed=false
  # Reuse only the already-sanitized environment name list.
  if [[ ! -f "$env_base" ]]; then cred_failed=true
  else
    while IFS=$'\t' read -r env_name env_path; do
      env_secrets="$tmp/secrets-$(printf '%s' "$env_name" | shasum | cut -d' ' -f1).json"
      if ! project "repos/$repo/environments/$env_path/secrets?per_page=100" 'if type=="object" and (.total_count|type)=="number" and (.secrets|type)=="array" then {total_count,secrets:[.secrets[]|{name,created_at,updated_at}]|sort_by(.name)} else error("shape") end' "$env_secrets"; then cred_failed=true; break; fi
      jq --arg name "$env_name" --slurpfile value "$env_secrets" '.environments += [($value[0]+{name:$name})]|.environments|=sort_by(.name)' "$tmp/credentials.json" >"$tmp/credentials.next"
      mv "$tmp/credentials.next" "$tmp/credentials.json"
    done < <(jq -r '.[]|[.name,(.name|@uri)]|@tsv' "$env_base")
  fi
  if [[ "$cred_failed" == true ]]; then failure credential-metadata; else observed credential-metadata "$tmp/credentials.json"; fi
fi

control immutable-releases "repos/$repo/immutable-releases" 'if type=="object" and (.enabled|type)=="boolean" and (.enforced_by_owner|type)=="boolean" then {valid:true,actual:{enabled,enforced_by_owner}} else {valid:false,actual:null} end'
control workflow-identities "repos/$repo/actions/workflows?per_page=100" 'if type=="object" and (.workflows|type)=="array" then {valid:true,actual:[.workflows[]|{name,path,state}]|sort_by(.path,.name)} else {valid:false,actual:null} end'
control latest-release "repos/$repo/releases/latest" 'if type=="object" and (.tag_name|type)=="string" and (.draft|type)=="boolean" and (.prerelease|type)=="boolean" then {valid:true,actual:{tag_name,draft,prerelease,immutable:(.immutable//false),published_at,author:(.author.login//null),asset_count:(.assets//[]|length)}} else {valid:false,actual:null} end'

mkdir -p "$(dirname "$output")"
jq -nS --arg repository "$repo" --arg ref "$ref" --arg commit "$commit" \
  --arg workflow "$workflow_sha" --arg collected_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --slurpfile controls "$controls" \
  '{schema_version:1,identity:{repository:$repository,ref:$ref,commit_sha:$commit,workflow_sha:$workflow,collected_at:$collected_at},controls:($controls|sort_by(.id))}' >"$output"
