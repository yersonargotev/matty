export const MATTY_PACKAGE_VERSION = "0.1.0";
export const STARTUP_HINT =
  "Matty active · /skill:ask-matt · /matty status";

export const CERTIFIED_PI_VERSIONS = ["0.83.0"] as const;
export const CERTIFIED_TARGETS = ["darwin/arm64"] as const;

export const SHARED_SKILL_CATALOG_MEMBER_COUNT = 22;
export const ACTIVATION_SAFETY_GATE: {
  state: "passed" | "blocked";
  issue: number;
} = {
  state: "blocked",
  issue: 3,
};
