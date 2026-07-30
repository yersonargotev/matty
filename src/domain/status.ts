import {
  INSPECTION_CAPABILITY_CONTRACTS,
  MATTY_ROLES,
  type MattyRole,
} from "./capability-contract.ts";
import {
  CERTIFIED_PI_VERSIONS,
  CERTIFIED_TARGETS,
  isCertifiedHost,
} from "./package-contract.ts";
import {
  WEB_CAPABILITY_TOOLS,
  type WebCapabilityState,
} from "./web-capability.ts";

export const REFERENCE_MODEL_PATH = {
  provider: "openai-codex",
  model: "gpt-5.6-sol",
  authentication: "chatgpt-codex-subscription",
} as const;

export type DiagnosticFailureSource =
  | "runtime-launch"
  | "role-data"
  | "rule-injection"
  | "capability-contract"
  | "web-integration"
  | "dependency"
  | "artifact-integrity";

export interface RuntimeFacts {
  packageVersion: string;
  piVersion: string;
  platform: NodeJS.Platform;
  arch: string;
  activeModel?: {
    provider: string;
    model: string;
    authentication?: "chatgpt-codex-subscription";
  };
  subagentRuntimeAvailable?: boolean;
  web: {
    state: WebCapabilityState;
    registeredTools: readonly string[];
  };
  failures?: readonly {
    source: DiagnosticFailureSource;
    error?: unknown;
  }[];
  concurrency?: {
    activeChildren: number;
    queuedChildren: number;
  };
  activation: {
    state: "active" | "degraded";
    reason: "compatible" | "unsupported-host";
    codes: readonly DiagnosticCode[];
  };
}

export type DiagnosticCode =
  | "host-uncertified"
  | "reference-model-unverified"
  | "subagent-runtime-unavailable"
  | "role-data-invalid"
  | "matty-rules-unavailable"
  | "capability-contract-invalid"
  | "web-capability-degraded"
  | "web-capability-unavailable"
  | "dependency-unavailable"
  | "artifact-integrity-failed";

export interface RedactedDiagnosticEntry {
  code: DiagnosticCode;
  severity: "warning" | "error";
  remediation: string;
}

export interface RedactedDiagnosticSnapshot {
  schemaVersion: 1;
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
  referenceModelPath: typeof REFERENCE_MODEL_PATH & {
    state: "verified" | "unverified";
  };
  activation: {
    state: "active" | "degraded";
    reason: "compatible" | "unsupported-host";
    codes: DiagnosticCode[];
  };
  subagentRuntime: {
    state: "available" | "unavailable";
    processIsolation: "child-process";
  };
  roles: {
    state: "available" | "unavailable";
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
  mattyRules: {
    schemaVersion: 1;
    state: "active" | "unavailable";
  };
  capabilityContracts: {
    schemaVersion: 1;
    state: "available" | "unavailable";
    ids: string[];
  };
  concurrency: {
    maxTasksPerCall: 8;
    maxActiveChildren: 4;
    singleWriter: true;
    activeChildren: number;
    queuedChildren: number;
  };
  web: {
    state: WebCapabilityState;
    tools: string[];
  };
  diagnostics: RedactedDiagnosticEntry[];
}

export type StatusDiagnostic = RedactedDiagnosticSnapshot & {
  command: "status";
};

export type DoctorDiagnostic = RedactedDiagnosticSnapshot & {
  command: "doctor";
};

const FAILURE_DIAGNOSTICS = {
  "runtime-launch": {
    code: "subagent-runtime-unavailable",
    severity: "error",
    remediation: "Verify the certified Pi executable can launch child processes.",
  },
  "role-data": {
    code: "role-data-invalid",
    severity: "error",
    remediation: "Reinstall the complete Matty package.",
  },
  "rule-injection": {
    code: "matty-rules-unavailable",
    severity: "error",
    remediation: "Remove conflicting Matty Rules markers and reload the session.",
  },
  "capability-contract": {
    code: "capability-contract-invalid",
    severity: "error",
    remediation: "Reinstall the complete Matty package.",
  },
  "web-integration": {
    code: "web-capability-unavailable",
    severity: "warning",
    remediation: "Verify the pinned Web Capability is installed and locally loadable.",
  },
  dependency: {
    code: "dependency-unavailable",
    severity: "error",
    remediation: "Reinstall Matty with its production dependencies.",
  },
  "artifact-integrity": {
    code: "artifact-integrity-failed",
    severity: "error",
    remediation: "Replace Matty with a verified published artifact.",
  },
} as const satisfies Record<
  DiagnosticFailureSource,
  RedactedDiagnosticEntry
>;

const CAPABILITY_CONTRACT_IDS = [
  ...Object.values(INSPECTION_CAPABILITY_CONTRACTS).map((contract) =>
    contract.id
  ),
  "delegate-researcher",
  "delegate-worker",
  "delegate-group",
  "parent-web",
] as const;

const DIAGNOSTIC_ORDER: readonly DiagnosticCode[] = [
  "host-uncertified",
  "reference-model-unverified",
  "subagent-runtime-unavailable",
  "role-data-invalid",
  "matty-rules-unavailable",
  "capability-contract-invalid",
  "web-capability-degraded",
  "web-capability-unavailable",
  "dependency-unavailable",
  "artifact-integrity-failed",
];

function addDiagnostic(
  diagnostics: RedactedDiagnosticEntry[],
  diagnostic: RedactedDiagnosticEntry,
): void {
  if (!diagnostics.some((entry) => entry.code === diagnostic.code)) {
    diagnostics.push(diagnostic);
  }
}

export function createDiagnosticSnapshot(
  facts: RuntimeFacts,
): RedactedDiagnosticSnapshot {
  const target = `${facts.platform}/${facts.arch}`;
  const certifiedHost = isCertifiedHost(
    facts.piVersion,
    facts.platform,
    facts.arch,
  );
  const verifiedModel =
    facts.activeModel?.provider === REFERENCE_MODEL_PATH.provider &&
    facts.activeModel.model === REFERENCE_MODEL_PATH.model &&
    facts.activeModel.authentication === REFERENCE_MODEL_PATH.authentication;
  const runtimeAvailable = facts.subagentRuntimeAvailable ?? true;
  const diagnostics: RedactedDiagnosticEntry[] = [];

  if (!certifiedHost) {
    addDiagnostic(diagnostics, {
      code: "host-uncertified",
      severity: "error",
      remediation:
        `Use Pi ${CERTIFIED_PI_VERSIONS[0]} on ${CERTIFIED_TARGETS[0]}, or treat this host as uncertified.`,
    });
  }
  if (!verifiedModel) {
    addDiagnostic(diagnostics, {
      code: "reference-model-unverified",
      severity: "warning",
      remediation:
        "Use the Reference Model Path for the certified path, or continue with an unverified model.",
    });
  }
  if (!runtimeAvailable) {
    addDiagnostic(diagnostics, FAILURE_DIAGNOSTICS["runtime-launch"]);
  }
  if (facts.web.state !== "available") {
    addDiagnostic(diagnostics, {
      code: facts.web.state === "degraded"
        ? "web-capability-degraded"
        : "web-capability-unavailable",
      severity: "warning",
      remediation:
        "Verify the pinned Web Capability and its four certified tools are locally loadable.",
    });
  }
  for (const failure of facts.failures ?? []) {
    const source = typeof failure === "object" && failure !== null
      ? (failure as { source?: unknown }).source
      : undefined;
    if (
      typeof source === "string" &&
      Object.hasOwn(FAILURE_DIAGNOSTICS, source)
    ) {
      addDiagnostic(
        diagnostics,
        FAILURE_DIAGNOSTICS[source as DiagnosticFailureSource],
      );
    }
  }
  diagnostics.sort((left, right) =>
    DIAGNOSTIC_ORDER.indexOf(left.code) -
    DIAGNOSTIC_ORDER.indexOf(right.code)
  );

  return {
    schemaVersion: 1,
    package: {
      name: "@yargote/matty",
      version: facts.packageVersion,
    },
    pi: {
      version: facts.piVersion,
      certifiedVersions: [...CERTIFIED_PI_VERSIONS],
      state: CERTIFIED_PI_VERSIONS.some((version) =>
          version === facts.piVersion
        )
        ? "certified"
        : "unsupported",
    },
    target: {
      platform: facts.platform,
      arch: facts.arch,
      certifiedTargets: [...CERTIFIED_TARGETS],
      state: CERTIFIED_TARGETS.some((certifiedTarget) =>
          certifiedTarget === target
        )
        ? "certified"
        : "unsupported",
    },
    referenceModelPath: {
      ...REFERENCE_MODEL_PATH,
      state: verifiedModel ? "verified" : "unverified",
    },
    activation: {
      state: facts.activation.state,
      reason: facts.activation.reason,
      codes: [...facts.activation.codes],
    },
    subagentRuntime: {
      state: runtimeAvailable ? "available" : "unavailable",
      processIsolation: "child-process",
    },
    roles: {
      state: diagnostics.some((entry) => entry.code === "role-data-invalid")
        ? "unavailable"
        : "available",
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
    mattyRules: {
      schemaVersion: 1,
      state: diagnostics.some((entry) =>
          entry.code === "matty-rules-unavailable"
        )
        ? "unavailable"
        : "active",
    },
    capabilityContracts: {
      schemaVersion: 1,
      state: diagnostics.some((entry) =>
          entry.code === "capability-contract-invalid"
        )
        ? "unavailable"
        : "available",
      ids: [...CAPABILITY_CONTRACT_IDS],
    },
    concurrency: {
      maxTasksPerCall: 8,
      maxActiveChildren: 4,
      singleWriter: true,
      activeChildren: facts.concurrency?.activeChildren ?? 0,
      queuedChildren: facts.concurrency?.queuedChildren ?? 0,
    },
    web: {
      state: facts.web.state,
      tools: facts.web.registeredTools.filter((tool) =>
        WEB_CAPABILITY_TOOLS.some((certifiedTool) => certifiedTool === tool)
      ),
    },
    diagnostics,
  };
}

export function createStatusSnapshot(
  facts: RuntimeFacts,
): StatusDiagnostic {
  return {
    ...createDiagnosticSnapshot(facts),
    command: "status",
  };
}

export function createStatusDiagnostic(
  snapshot: RedactedDiagnosticSnapshot,
): StatusDiagnostic {
  return { ...snapshot, command: "status" };
}

export function createDoctorDiagnostic(
  snapshot: RedactedDiagnosticSnapshot,
): DoctorDiagnostic {
  return { ...snapshot, command: "doctor" };
}

export function renderStatusHuman(
  snapshot: RedactedDiagnosticSnapshot,
): string {
  return [
    `Matty ${snapshot.package.version}`,
    `Host Pi ${snapshot.pi.version} · ${snapshot.pi.state}`,
    `Target ${snapshot.target.platform}/${snapshot.target.arch} · ${snapshot.target.state}`,
    `Reference Model Path ${snapshot.referenceModelPath.provider}/${snapshot.referenceModelPath.model} · ${snapshot.referenceModelPath.state}`,
    `Activation ${snapshot.activation.state} · ${snapshot.activation.reason}`,
    `Subagent Runtime ${snapshot.subagentRuntime.state} · ${snapshot.subagentRuntime.processIsolation}`,
    `Roles ${snapshot.roles.state} · ${snapshot.roles.available.join(", ")}`,
    "Inspection Guard best-effort · not a security sandbox",
    "Worker Guard best-effort · Single Writer · not a security sandbox",
    `Matty Rules v${snapshot.mattyRules.schemaVersion} · ${snapshot.mattyRules.state}`,
    `Capability Contracts v${snapshot.capabilityContracts.schemaVersion} · ${snapshot.capabilityContracts.state} · ${snapshot.capabilityContracts.ids.join(", ")}`,
    `Concurrency ${snapshot.concurrency.maxTasksPerCall} accepted · ${snapshot.concurrency.maxActiveChildren} max active · ${snapshot.concurrency.activeChildren} active · ${snapshot.concurrency.queuedChildren} queued · Single Writer`,
    `Web Capability ${snapshot.web.state} · ${
      snapshot.web.tools.length > 0
        ? snapshot.web.tools.join(", ")
        : "no certified tools"
    }`,
  ].join("\n");
}

export function renderDoctorHuman(
  snapshot: RedactedDiagnosticSnapshot,
): string {
  return [
    `Matty doctor · ${snapshot.activation.state}`,
    ...(snapshot.diagnostics.length === 0
      ? ["No remediation required."]
      : snapshot.diagnostics.map((diagnostic, index) =>
        `${index + 1}. ${diagnostic.code} · ${diagnostic.remediation}`
      )),
  ].join("\n");
}

export function renderStatusJson(snapshot: RedactedDiagnosticSnapshot): string {
  return JSON.stringify(createStatusDiagnostic(snapshot));
}

export function renderDoctorJson(snapshot: RedactedDiagnosticSnapshot): string {
  return JSON.stringify(createDoctorDiagnostic(snapshot));
}
