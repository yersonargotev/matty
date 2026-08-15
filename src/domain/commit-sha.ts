declare const commitShaBrand: unique symbol;

/** A full, lowercase Git object identity accepted at untrusted boundaries. */
export type CommitSha = string & { readonly [commitShaBrand]: "CommitSha" };

export function isCommitSha(value: unknown): value is CommitSha {
  return typeof value === "string" && /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/.test(value);
}

export function commitSha(value: unknown): CommitSha {
  if (!isCommitSha(value)) {
    throw new Error("invalid commit SHA");
  }
  return value as CommitSha;
}

export function abbreviatedCommitSha(value: CommitSha): string {
  return value.slice(0, 12);
}
