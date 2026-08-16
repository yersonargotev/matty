export const MATTY_PACKAGE_VERSION = "0.2.0";
export const STARTUP_HINT = "Matty active · /matty status";

export const CERTIFIED_PI_VERSIONS = ["0.84.2"] as const;
export const CERTIFIED_TARGETS = ["darwin/arm64"] as const;

export function isCertifiedHost(
  piVersion: string,
  platform: NodeJS.Platform,
  arch: string,
): boolean {
  const target = `${platform}/${arch}`;
  return (
    CERTIFIED_PI_VERSIONS.some((certified) => certified === piVersion) &&
    CERTIFIED_TARGETS.some((certified) => certified === target)
  );
}
