import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = new URL("../../", import.meta.url);
const shippedPath = new URL("skills/gh/SKILL.md", root);
const adHocRoot = new URL("evals/skills/", root);

test("ad hoc eval skills are staged under their own name", async () => {
  let variants: string[];
  try {
    variants = (await readdir(adHocRoot, { withFileTypes: true })).filter(entry => entry.isDirectory()).map(entry => entry.name);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return;
    throw error;
  }
  for (const variant of variants) {
    for (const skill of await readdir(new URL(`${variant}/`, adHocRoot), { withFileTypes: true })) {
      assert.ok(skill.isDirectory(), `evals/skills/${variant}/${skill.name} must be a skill directory`);
      const text = await readFile(new URL(`${variant}/${skill.name}/SKILL.md`, adHocRoot), "utf8");
      assert.match(text, new RegExp(`^---\\n(?:.*\\n)*?name: ${skill.name}\\n`),
        `evals/skills/${variant}/${skill.name}/SKILL.md must declare name: ${skill.name}`);
    }
  }
});

test("the shipped gh skill documents every dedicated event route the events suite relies on", async () => {
  const skill = await readFile(shippedPath, "utf8");
  for (const command of [
    "gh event listen --resource", "--event com.github.pull_request.review.submitted.v1 --timeout", "--event com.github.pull_request.review.submitted.v1,com.github.pull_request.state.changed.v1 --follow",
  ]) {
    assert.ok(skill.includes(command), `Missing command guidance in ${fileURLToPath(shippedPath)}: ${command}`);
  }
  assert.doesNotMatch(skill, /\/event-subscriptions/, "the skill must not teach the superseded gh api subscription recipe");
  assert.doesNotMatch(skill, /--after-review\b/, "the skill must not teach the removed review cursor");
  assert.match(skill, /Do not launch the stream and initial snapshot in the same parallel batch/,
    "the skill must explicitly close the live-only startup race exposed by the eval");
  assert.match(skill, /Wait until stderr reports that the listener is listening for events/,
    "the skill must use the command's deterministic listener-ready signal");
});
