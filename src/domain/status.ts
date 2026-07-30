import {
  CERTIFIED_PI_VERSIONS,
  CERTIFIED_TARGETS,
} from "./package-contract.ts";
import {
  MATTY_ROLES,
  type MattyRole,
} from "./capability-contract.ts";

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
  activation: {
    state: "active" | "degraded";
    reason: "compatible" | "unsupported-host";
  };
  roles: {
    available: MattyRole[];
  };
  inspectionGuard: {
    state: "best-effort";
    securityBoundary: false;
  };
  workerGuard: {
    state: "best-effort";
    securityBoundary: false;
    singleWriter: true;
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
    activation: {
      state: compatibleHost ? "active" : "degraded",
      reason: compatibleHost ? "compatible" : "unsupported-host",
    },
    roles: {
      available: [...MATTY_ROLES],
    },
    inspectionGuard: {
      state: "best-effort",
      securityBoundary: false,
    },
    workerGuard: {
      state: "best-effort",
      securityBoundary: false,
      singleWriter: true,
    },
  };
}

export function renderStatusHuman(snapshot: StatusDiagnostic): string {
  return [
    `Matty ${snapshot.package.version}`,
    `Pi ${snapshot.pi.version} · ${snapshot.pi.state}`,
    `Target ${snapshot.target.platform}/${snapshot.target.arch} · ${snapshot.target.state}`,
    `Activation ${snapshot.activation.state} · ${snapshot.activation.reason}`,
    `Roles ${snapshot.roles.available.join(", ")}`,
    "Inspection Guard best-effort · not a security sandbox",
    "Worker Guard best-effort · Single Writer · not a security sandbox",
  ].join("\n");
}

export function renderStatusJson(snapshot: StatusDiagnostic): string {
  return JSON.stringify(snapshot);
}
