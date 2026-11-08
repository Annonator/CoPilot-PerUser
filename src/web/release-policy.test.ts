// @vitest-environment node

import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// ESLint already installs this parser through the committed package-lock.json.
const { load } = createRequire(import.meta.url)("js-yaml") as {
  load: (source: string) => Workflow;
};
type Step = { name: string; uses?: string; run?: string; with?: Record<string, unknown>; env?: Record<string, string> };
type Job = { if?: string; needs?: string | string[]; permissions?: Record<string, string>; steps: Step[]; outputs?: Record<string, string> };
type Workflow = { on: Record<string, unknown>; permissions: Record<string, string>; jobs: Record<string, Job> };
const workflow = load(readFileSync(fileURLToPath(new URL("../../.github/workflows/release.yml", import.meta.url)), "utf8"));

describe("official release trust boundary", () => {
  it("requires dispatch from main before downstream jobs can run", () => {
    expect(Object.keys(workflow.on)).toEqual(["workflow_dispatch"]);
    expect(workflow.jobs.validate.if).toBe("github.event_name == 'workflow_dispatch' && github.ref == 'refs/heads/main'");
    expect(workflow.permissions).toEqual({ contents: "read" });
    for (const name of ["backend", "frontend", "publish", "release"]) {
      expect([workflow.jobs[name].needs].flat()).toContain("validate");
    }
  });

  it("checks out only the validated immutable commit with no stored credential", () => {
    for (const [name, job] of Object.entries(workflow.jobs)) {
      const checkout = job.steps.find((step) => step.uses?.startsWith("actions/checkout@"));
      expect(checkout?.with?.ref).toBe(name === "validate" ? "${{ github.sha }}" : "${{ needs.validate.outputs.sha }}");
      expect(checkout?.with?.["persist-credentials"]).toBe(false);
    }
  });

  it("revalidates the tag, main commit, and server policy before each publishing sink", () => {
    for (const name of ["publish", "release"]) {
      const job = workflow.jobs[name];
      const verifyIndex = job.steps.findIndex((step) => step.run === "bash scripts/verify-release.sh");
      const sinkIndex = job.steps.findIndex((step) => step.uses?.startsWith("docker/build-push-action@") || step.run?.startsWith("gh release create "));
      expect(verifyIndex).toBeGreaterThanOrEqual(0);
      expect(sinkIndex).toBeGreaterThan(verifyIndex);
      expect(job.steps[verifyIndex].env).toMatchObject({
        RELEASE_TAG: "${{ needs.validate.outputs.tag }}", EXPECTED_RELEASE_SHA: "${{ needs.validate.outputs.sha }}",
      });
    }
    expect(workflow.jobs.publish.permissions).toEqual({ contents: "read", packages: "write" });
    expect(workflow.jobs.release.permissions).toEqual({ contents: "write" });
    expect(workflow.jobs.release.steps.find((step) => step.run?.startsWith("gh release create "))?.run).toContain("--verify-tag");
  });

  it("uses validated tag metadata and passes caller input through environment variables", () => {
    const metadata = workflow.jobs.publish.steps.find((step) => step.uses?.startsWith("docker/metadata-action@"));
    expect(metadata?.with?.tags).toContain("value=${{ needs.validate.outputs.tag }}");
    expect(metadata?.with?.tags).toContain("value=${{ needs.validate.outputs.version }}");
    expect(metadata?.with?.tags).not.toContain("event=tag");
    for (const job of Object.values(workflow.jobs)) {
      for (const step of job.steps) {
        expect(step.run ?? "").not.toMatch(/\$\{\{\s*(inputs\.|github\.event\.inputs\.)/);
      }
    }
  });
});
