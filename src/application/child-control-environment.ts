/** Internal one-hop transport keys. These are never repository command inputs. */
export const CHILD_CONTROL_ENVIRONMENT_NAMES = [
  "MATTY_CHILD_ROLE",
  "MATTY_RESEARCH_CONTRACT",
  "MATTY_RESEARCH_SCOPE",
  "MATTY_WORKER_WORKING_TREE",
  "MATTY_WORKER_TEMPORARY_PATHS",
  "MATTY_WORKER_PROTECTED_PATHS",
  "MATTY_WORKER_USER_HOME",
  "MATTY_WORKER_USER_CONFIGURATION_PATHS",
] as const;

export type ChildControlEnvironmentName = typeof CHILD_CONTROL_ENVIRONMENT_NAMES[number];

export const CHILD_CONTROL_ENVIRONMENT = {
  role: "MATTY_CHILD_ROLE",
  researchContract: "MATTY_RESEARCH_CONTRACT",
  researchScope: "MATTY_RESEARCH_SCOPE",
  workerWorkingTree: "MATTY_WORKER_WORKING_TREE",
  workerTemporaryPaths: "MATTY_WORKER_TEMPORARY_PATHS",
  workerProtectedPaths: "MATTY_WORKER_PROTECTED_PATHS",
  workerUserHome: "MATTY_WORKER_USER_HOME",
  workerUserConfigurationPaths: "MATTY_WORKER_USER_CONFIGURATION_PATHS",
} as const satisfies Record<string, ChildControlEnvironmentName>;

export function scrubChildControlEnvironment(environment: NodeJS.ProcessEnv): void {
  for (const name of CHILD_CONTROL_ENVIRONMENT_NAMES) delete environment[name];
}
