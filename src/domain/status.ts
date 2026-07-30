import {
  ACTIVATION_SAFETY_GATE,
  CERTIFIED_PI_VERSIONS,
  CERTIFIED_TARGETS,
  SHARED_SKILL_CATALOG_MEMBER_COUNT,
} from "./package-contract.ts";

export interface RuntimeFacts {
  packageVersion: string;
  piVersion: string;
  platform: NodeJS.Platform;
  arch: string;
}

export interface StatusDiagnostic {
  schemaVersion: 1;
  command: "status";
  package: {
    name: "@yargote/matty";
    version: string;
  };
  pi: {
    version: string;
    certifiedVersions: string[];
    state: "certified" | "unsupported";
  };
  target: {
    platform: NodeJS.Platform;
    arch: string;
    certifiedTargets: string[];
    state: "certified" | "unsupported";
  };
  catalog: {
    memberCount: number;
    state: "staged";
    activationSafetyGate: "passed" | "blocked";
    blockingIssue: number;
  };
  activation: {
    state: "active" | "degraded";
    reason:
      | "compatible"
      | "unsupported-host"
      | "activation-safety-gate";
  };
}

export function createStatusSnapshot(
  facts: RuntimeFacts,
): StatusDiagnostic {
  const target = `${facts.platform}/${facts.arch}`;
  const piState = CERTIFIED_PI_VERSIONS.some(
    (version) => version === facts.piVersion,
  )
    ? "certified"
    : "unsupported";
  const targetState = CERTIFIED_TARGETS.some(
    (certifiedTarget) => certifiedTarget === target,
  )
    ? "certified"
    : "unsupported";
  const compatibleHost =
    piState === "certified" && targetState === "certified";
  const active =
    compatibleHost && ACTIVATION_SAFETY_GATE.state !== "blocked";

  return {
    schemaVersion: 1,
    command: "status",
    package: {
      name: "@yargote/matty",
      version: facts.packageVersion,
    },
    pi: {
      version: facts.piVersion,
      certifiedVersions: [...CERTIFIED_PI_VERSIONS],
      state: piState,
    },
    target: {
      platform: facts.platform,
      arch: facts.arch,
      certifiedTargets: [...CERTIFIED_TARGETS],
      state: targetState,
    },
    catalog: {
      memberCount: SHARED_SKILL_CATALOG_MEMBER_COUNT,
      state: "staged",
      activationSafetyGate: ACTIVATION_SAFETY_GATE.state,
      blockingIssue: ACTIVATION_SAFETY_GATE.issue,
    },
    activation: {
      state: active ? "active" : "degraded",
      reason: active
        ? "compatible"
        : compatibleHost
        ? "activation-safety-gate"
        : "unsupported-host",
    },
  };
}

export function renderStatusHuman(snapshot: StatusDiagnostic): string {
  return [
    `Matty ${snapshot.package.version}`,
    `Pi ${snapshot.pi.version} · ${snapshot.pi.state}`,
    `Target ${snapshot.target.platform}/${snapshot.target.arch} · ${snapshot.target.state}`,
    `Catalog ${snapshot.catalog.memberCount} skills · ${snapshot.catalog.state}`,
    `Activation ${snapshot.activation.state} · ${snapshot.activation.reason}`,
  ].join("\n");
}

export function renderStatusJson(snapshot: StatusDiagnostic): string {
  return JSON.stringify(snapshot);
}
