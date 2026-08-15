import { createHash } from "node:crypto";
import { basename, dirname, join } from "node:path";

import type {
  DeliveryIdentity,
  DeliveryWorkspace,
  ExceptionBrief,
} from "../domain/issue-delivery.ts";
import type {
  ExistingIssueDeliveryResult,
  IssueDeliveryWorkspace,
  IssueDeliveryWorkspaceRequest,
  IssueDeliveryWorkspaceResult,
} from "./issue-delivery.ts";

export interface WorkspaceCheckoutFacts {
  root: string;
  ref: string | null;
  sha: string;
  clean: boolean;
  integrationBranch: string;
  integrationSha: string;
}

export interface DeliveryOwnershipRecord {
  schemaVersion: 1;
  status: "active";
  key: string;
  identity: DeliveryIdentity;
  branch: string;
  path: string;
  isolation: "in-place" | "worktree";
  startingCheckout: DeliveryWorkspace["startingCheckout"];
  integration: {
    branch: string;
    sha: string;
  };
}

export interface IssueDeliveryWorkspacePort {
  inspect(cwd: string): Promise<WorkspaceCheckoutFacts>;
  inspectActive(
    cwd: string,
    issue: number,
  ): Promise<ExistingIssueDeliveryResult>;
  readActive(root: string): Promise<DeliveryOwnershipRecord | undefined>;
  inspectOwnership(
    record: DeliveryOwnershipRecord,
  ): Promise<"absent" | "owned" | "mismatch">;
  claim(
    record: DeliveryOwnershipRecord,
  ): Promise<"claimed" | "same" | "different">;
  prepare(record: DeliveryOwnershipRecord): Promise<void>;
}

export function deliveryIdentityKey(identity: DeliveryIdentity): string {
  return createHash("sha256").update(
    `${identity.repository.toLowerCase()}\n${identity.tracker}\n${identity.issue}`,
  ).digest("hex");
}

function blocked(
  evidence: "delivery-active" | "delivery-ownership-mismatch",
  issue: number,
): IssueDeliveryWorkspaceResult {
  const active = evidence === "delivery-active";
  const exceptionBrief: ExceptionBrief = {
    schemaVersion: 1,
    gate: "workspace-preparation",
    evidence: [evidence],
    need: active
      ? "A different nonterminal Issue Delivery already owns this repository."
      : "Delivery branch or worktree ownership could not be verified from its durable record.",
    options: active
      ? ["Finish or cancel the active delivery before starting another issue."]
      : [`Remove or explicitly reconcile the conflicting resource, then repeat /matty deliver ${issue}.`],
    recommendation: active
      ? "Preserve both checkouts and continue the marked active delivery."
      : "Do not infer ownership from branch or worktree names.",
  };
  return { status: "blocked", exceptionBrief };
}

function proposedRecord(
  request: IssueDeliveryWorkspaceRequest,
  facts: WorkspaceCheckoutFacts,
): DeliveryOwnershipRecord {
  const key = deliveryIdentityKey(request.identity);
  const branch = `matty/deliver-${request.identity.issue}-${key.slice(0, 8)}`;
  const inPlace = facts.clean && facts.ref === facts.integrationBranch &&
    facts.sha === facts.integrationSha;
  return {
    schemaVersion: 1,
    status: "active",
    key,
    identity: request.identity,
    branch,
    path: inPlace
      ? facts.root
      : join(
        dirname(facts.root),
        ".matty-worktrees",
        basename(facts.root),
        key.slice(0, 12),
      ),
    isolation: inPlace ? "in-place" : "worktree",
    startingCheckout: {
      root: facts.root,
      ref: facts.ref,
      sha: facts.sha,
    },
    integration: {
      branch: facts.integrationBranch,
      sha: facts.integrationSha,
    },
  };
}

function prepared(
  record: DeliveryOwnershipRecord,
  resumed: boolean,
): IssueDeliveryWorkspaceResult {
  return {
    status: "prepared",
    workspace: {
      root: record.startingCheckout.root,
      path: record.path,
      branch: record.branch,
      isolation: record.isolation,
      resumed,
      startingCheckout: record.startingCheckout,
    },
  };
}

export function createIssueDeliveryWorkspace(
  port: IssueDeliveryWorkspacePort,
): IssueDeliveryWorkspace {
  return {
    async inspect(request) {
      return await port.inspectActive(request.cwd, request.issue);
    },
    async prepare(request) {
      const facts = await port.inspect(request.cwd);
      const proposal = proposedRecord(request, facts);
      const active = await port.readActive(facts.root);
      if (active && active.key !== deliveryIdentityKey(active.identity)) {
        return blocked("delivery-ownership-mismatch", request.identity.issue);
      }
      if (active && active.key !== proposal.key) {
        return blocked("delivery-active", request.identity.issue);
      }

      const record = active ?? proposal;
      const ownership = await port.inspectOwnership(record);
      if (ownership === "mismatch") {
        return blocked("delivery-ownership-mismatch", request.identity.issue);
      }
      if (!active && ownership === "owned" && record.key !== proposal.key) {
        return blocked("delivery-ownership-mismatch", request.identity.issue);
      }

      const claim = await port.claim(record);
      if (claim === "different") {
        return blocked("delivery-active", request.identity.issue);
      }
      await port.prepare(record);
      return prepared(record, active !== undefined || claim === "same" || ownership === "owned");
    },
  };
}
