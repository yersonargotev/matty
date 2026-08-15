import assert from "node:assert/strict";
import test from "node:test";

import {
  ISSUE_DELIVERY_GUIDANCE_END,
  ISSUE_DELIVERY_GUIDANCE_START,
  injectIssueDeliveryGuidance,
} from "../src/domain/workflow-guidance.ts";

test("package-owned Workflow Guidance is versioned and deduplicated", () => {
  const once = injectIssueDeliveryGuidance("Repository instructions");
  const twice = injectIssueDeliveryGuidance(once);

  assert.equal(twice, once);
  assert.match(once, /Issue Delivery Workflow Guidance v1/);
  assert.equal(once.split(ISSUE_DELIVERY_GUIDANCE_START).length - 1, 1);
  assert.equal(once.split(ISSUE_DELIVERY_GUIDANCE_END).length - 1, 1);
  assert.match(once, /Repository instructions/);
  assert.match(once, /\/matty deliver <issue>/);
  assert.match(once, /ask-matt.*do(?:es)? not authorize/i);
});
