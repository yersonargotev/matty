import { readFileSync } from "node:fs";

export const ISSUE_DELIVERY_GUIDANCE_START =
  "<!-- matty:issue-delivery-guidance -->";
export const ISSUE_DELIVERY_GUIDANCE_END =
  "<!-- /matty:issue-delivery-guidance -->";

const guidance = readFileSync(
  new URL("./issue-delivery-guidance-v1.md", import.meta.url),
  "utf8",
).trim();

function withoutIssueDeliveryGuidance(systemPrompt: string): string {
  return systemPrompt
    .replace(
      /<!-- matty:issue-delivery-guidance -->[\s\S]*?<!-- \/matty:issue-delivery-guidance -->/g,
      "",
    )
    .replaceAll(ISSUE_DELIVERY_GUIDANCE_START, "")
    .replaceAll(ISSUE_DELIVERY_GUIDANCE_END, "")
    .trim();
}

export function injectIssueDeliveryGuidance(systemPrompt: string): string {
  return [withoutIssueDeliveryGuidance(systemPrompt), guidance]
    .filter(Boolean)
    .join("\n\n");
}
