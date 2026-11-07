const assert = require("node:assert/strict");
const { test } = require("node:test");
const { parseReport, publishReport, ghClient, wordCount, mark } = require("./issue-triage-report.cjs");

const context = { repo: "octo/hello", issue: 12, runId: "123", workflow: "issue-triage", bot: "triage[bot]", staged: false };
const comment = "**Reproducible on 2.83.2.**\n\nUsing test data for a failed workflow run:\n\n```console\n$ gh run view 123 -R octo/hello --exit-status --log\ntest\ttest\tsimulated failure\n```\n\n**Exit status:** `0` (success). Expected: `1` (failure).";
const input = () => ({
  items: [
    { type: "add_labels", item_number: 12, labels: [
      { name: "bug", rationale: "The command returned success for a failed run.", confidence: "HIGH", suggest: true },
    ] },
    { type: "publish_triage", outcome: "reproduced", comment,
      duplicate_analysis: "Searched open and closed issues. The closest report concerned a different command.",
      reproduction_log: "Using test data for a failed run, the command printed its log and exited 0.\n\n```console\n$ gh run view 123 -R octo/hello --exit-status --log\ntest\ttest\tsimulated failure\n```",
      investigation_notes: "The log path returns before checking the run's result.",
    },
  ],
});
const identity = text => text;
const decision = data => data.items.find(item => item.type === "publish_triage");
const duplicateInput = () => {
  const data = input();
  data.items[0].labels = [{ name: "duplicate", rationale: "Both reports describe the same log flag behavior.", confidence: "HIGH", suggest: true }];
  Object.assign(decision(data), {
    outcome: "duplicate", comment: "", duplicate_issue: "10",
    duplicate_reason: "both reports describe a successful exit for failed-run logs",
    reproduction_log: "", investigation_notes: "",
  });
  return data;
};
const fixtureURL = "https://github.com/example.invalid/hello/pull/1";
const fixtureInput = () => {
  const data = input();
  Object.assign(decision(data), {
    outcome: "partial",
    comment: `**Partly reproduced on 2.46.0 and 2.103.3.**

Using test data, both builds created the pull request before requesting reviewers:

\`\`\`console
$ gh pr create -f -r monalisa
${fixtureURL}
\`\`\`

**Exit status:** \`0\`.

The test did not reproduce GitHub's automatic CODEOWNERS assignment.`,
    reproduction_log: `Synthetic repository: \`example.invalid/hello\`.

\`\`\`console
$ gh pr create -f -r monalisa
${fixtureURL}
\`\`\`

Both builds exited 0. Captured stdout: \`${fixtureURL}\`.`,
  });
  return data;
};

test("parseReport", async t => {
  const cases = [
    {
      name: "short reproduction and separate reports",
      check: report => {
        assert.equal(report.comment, comment);
        assert.equal(Object.keys(report.documents).length, 4);
        assert.match(report.documents.label_reasoning, /The command returned success for a failed run/);
        assert.ok(wordCount(report.comment) < 130);
      },
    },
    {
      name: "duplicate has only the required sentence and existing evidence",
      data: duplicateInput,
      check: report => {
        assert.equal(report.comment, "I believe this is a duplicate of [#10](https://github.com/cli/cli/issues/10) because both reports describe a successful exit for failed-run logs.");
        assert.deepEqual(Object.keys(report.documents), ["duplicate_analysis", "label_reasoning"]);
      },
    },
    {
      name: "duplicate reasons can name versions and domains without adding sentences",
      data: duplicateInput,
      change: data => decision(data).duplicate_reason = "both describe the same error on github.com with version 2.83.2",
      check: report => assert.match(report.comment, /github\.com with version 2\.83\.2\.$/),
    },
    {
      name: "not reproduced reports what happened",
      change: data => {
        decision(data).outcome = "not_reproduced";
        decision(data).comment = comment.replace("Reproducible on", "Not reproduced on");
      },
    },
    {
      name: "partial result",
      change: data => {
        decision(data).outcome = "partial";
        decision(data).comment = comment.replace("Reproducible on", "Partly reproduced on") + "\n\nThis does not test the live service.";
      },
    },
    {
      name: "blocked is not a negative reproduction",
      change: data => {
        decision(data).outcome = "blocked";
        decision(data).comment = "**Unable to test: the reported build is unavailable.**";
        decision(data).reproduction_log = "";
      },
    },
    {
      name: "other issue with no label suggestions",
      change: data => {
        data.items.shift();
        Object.assign(decision(data), { outcome: "other", comment: "This would add a new option.", reproduction_log: "", investigation_notes: "" });
      },
      check: report => assert.deepEqual(Object.keys(report.documents), ["duplicate_analysis"]),
    },
    {
      name: "no output is explicit",
      change: data => decision(data).comment = comment.replace("test\ttest\tsimulated failure\n", "").replace("**Exit status:**", "**Output:** none.\n\n**Exit status:**"),
    },
    {
      name: "plain no-output wording matches the workflow instructions",
      change: data => decision(data).comment = comment.replace("test\ttest\tsimulated failure\n", "").replace("**Exit status:**", "Output: none.\n\n**Exit status:**"),
    },
    {
      name: "stopped process has no exit status",
      change: data => {
        decision(data).outcome = "partial";
        decision(data).comment = comment.replace("Reproducible on", "Partly reproduced on").replace("`0` (success). Expected: `1` (failure).", "not available; the command was stopped while waiting for input.");
      },
    },
    { name: "sanitization is applied before rendering", sanitize: value => value.replace("closest", "nearest"),
      check: report => assert.match(report.documents.duplicate_analysis, /nearest/) },
    { name: "missing safe-output items", data: () => ({}), error: /Missing safe-output/ },
    { name: "no publication", change: data => data.items.pop(), error: /Exactly one/ },
    { name: "two publications", change: data => data.items.push({ ...decision(data) }), error: /Exactly one/ },
    { name: "no second comment", change: data => data.items.push({ type: "add_comment" }), error: /another comment/ },
    { name: "spam cannot publish", change: data => data.items.push({ type: "apply_suspected_spam" }), error: /spam action/ },
    { name: "unknown outcome", change: data => decision(data).outcome = "confirmed", error: /Invalid triage outcome/ },
    { name: "labels are suggestions", change: data => data.items[0].labels[0].suggest = false, error: /suggestions/ },
    { name: "wrong label issue", change: data => data.items[0].item_number = 99, error: /different issue/ },
    { name: "wrong label repository", change: data => data.items[0].repo = "another/repo", error: /different repository/ },
    { name: "label cap", change: data => data.items[0].labels = Array(4).fill(data.items[0].labels[0]), error: /At most three/ },
    { name: "repeated labels", change: data => data.items[0].labels.push({ ...data.items[0].labels[0] }), error: /repeat label/ },
    { name: "keep needs-triage", change: data => data.items[0].labels[0].name = "needs-triage", error: /Invalid label/ },
    { name: "duplicate-only labels", data: duplicateInput, change: data => data.items[0].labels[0].name = "bug", error: /only duplicate/ },
    { name: "duplicate stops source investigation", data: duplicateInput, change: data => decision(data).investigation_notes = "More research.", error: /Stop investigation/ },
    { name: "duplicate stops reproduction", data: duplicateInput, change: data => decision(data).reproduction_log = "Ran commands.", error: /Stop investigation/ },
    { name: "duplicate is another issue", data: duplicateInput, change: data => decision(data).duplicate_issue = "12", error: /another issue/ },
    { name: "duplicate reason is one sentence", data: duplicateInput, change: data => decision(data).duplicate_reason = "This is similar. Also another thing.", error: /one sentence/ },
    { name: "uncertain match cannot suggest duplicate", change: data => data.items[0].labels[0].name = "duplicate", error: /Only a confirmed duplicate/ },
    { name: "no backend in reports", change: data => decision(data).reproduction_log += "\nghmock run", error: /not the test backend/ },
    {
      name: "a backend name in a hidden link destination is not visible prose",
      change: data => decision(data).reproduction_log += "\n[Run evidence](https://github.com/octo/ghmock-trials/actions/runs/123)",
      check: report => assert.match(report.documents.reproduction_log, /\[Run evidence\]\(https:\/\/github.com\/octo\/ghmock-trials\/actions\/runs\/123\)/),
    },
    { name: "visible backend link labels are still rejected", change: data => decision(data).reproduction_log += "\n[ghmock](https://github.com/octo/hello)", error: /not the test backend/ },
    {
      name: "literal angle brackets in output are not hidden HTML",
      change: data => decision(data).comment = comment.replace("test\ttest\tsimulated failure", "error: expected <run-id>"),
      check: report => assert.match(report.comment, /error: expected <run-id>/),
    },
    { name: "no backend in labels", change: data => data.items[0].labels[0].rationale = "ghmock showed it", error: /not the test backend/ },
    { name: "no negative search prose", change: data => decision(data).comment += "\nNo duplicate found.", error: /negative duplicate/ },
    { name: "no label prose", change: data => decision(data).comment += "\nSuggesting labels for this.", error: /label reasoning/ },
    { name: "wrong result heading", change: data => decision(data).outcome = "not_reproduced", error: /selected reproduction outcome/ },
    { name: "include reproduction log", change: data => decision(data).reproduction_log = "", error: /reproduction log/ },
    { name: "command must be gh", change: data => decision(data).comment = comment.replace("$ gh ", "$ curl "), error: /actual gh command/ },
    { name: "include output", change: data => decision(data).comment = comment.replace("test\ttest\tsimulated failure\n", ""), error: /captured output/ },
    { name: "include exit status", change: data => decision(data).comment = comment.replace("**Exit status:**", "Status:"), error: /exit status separately/ },
    { name: "no hidden top-level text", change: data => decision(data).comment += "\n<details>More.</details>", error: /HTML/ },
    { name: "visible budget includes footer", change: data => decision(data).comment += "\n" + "word ".repeat(125), error: /150 words/ },
    { name: "one small code block", change: data => decision(data).comment += "\n```text\nmore\n```", error: /one five-line/ },
    { name: "trim output", change: data => decision(data).comment = comment.replace("test\ttest\tsimulated failure", "one\ntwo\nthree\nfour\nfive"), error: /five-line/ },
    { name: "balanced code", change: data => decision(data).comment += "\n```", error: /Close the code fence/ },
    { name: "no em dashes", change: data => decision(data).comment += "\nOne\u2014two.", error: /ordinary hyphens/ },
    { name: "no control characters", change: data => decision(data).comment += "\u001b[0m", error: /control characters/ },
    { name: "bounded report body", change: data => decision(data).duplicate_analysis = "x".repeat(10001), error: /10000 characters/ },
  ];
  for (const entry of cases) await t.test(entry.name, () => {
    const data = (entry.data ?? input)();
    entry.change?.(data);
    if (entry.error) assert.throws(() => parseReport(data, context, entry.sanitize ?? identity), entry.error);
    else {
      const report = parseReport(data, context, entry.sanitize ?? identity);
      entry.check?.(report);
    }
  });
});

function fakeAPI(options = {}) {
  const calls = [];
  const artifacts = structuredClone(options.artifacts ?? []);
  const comments = structuredClone(options.comments ?? []);
  let detailReads = 0;
  let commentReads = 0;
  const api = async (method, endpoint, data) => {
    calls.push({ method, endpoint, data });
    const failure = options.fail?.(method, endpoint, data, calls);
    if (failure) throw failure instanceof Error ? failure : new Error("Injected request failure");
    if (endpoint === "repos/octo/hello") return { private: options.private ?? true };
    if (endpoint === "repos/hidden/project") return { private: true };
    if (endpoint === "repos/public/project") return { private: false };
    if (endpoint === "repos/octo/hello/issues/12") return { state: options.closed ? "closed" : "open", locked: false, ...(options.pr ? { pull_request: {} } : {}) };
    if (method === "GET" && endpoint.includes("/artifacts?")) return structuredClone(artifacts);
    if (method === "GET" && /\/artifacts\/\d+$/.test(endpoint)) {
      detailReads++;
      const artifact = structuredClone(artifacts.find(a => a.number === Number(endpoint.split("/").at(-1))));
      if (options.concurrent && detailReads === 2) artifact.body = "Human edit";
      return { artifact, versions: [{ actor: { login: options.historicalHuman ? "human" : context.bot } }] };
    }
    if (["POST", "PATCH"].includes(method) && endpoint.includes("/artifacts")) {
      const number = method === "POST" ? artifacts.length + 1 : Number(endpoint.split("/").at(-1));
      const artifact = { type: "generic", ...data, number, html_url: `https://github.com/octo/hello/issues/12/artifacts/${number}`,
        creator: { login: context.bot }, updated_by_actor: { login: context.bot }, updated_at: "2026-01-01T00:00:00Z" };
      if (method === "POST") artifacts.push(artifact);
      else artifacts.splice(artifacts.findIndex(a => a.number === number), 1, artifact);
      if (options.lostArtifactResponse) throw new Error("Response lost after write");
      return structuredClone(artifact);
    }
    if (method === "GET" && endpoint.includes("/comments?")) {
      if (options.concurrentComment && ++commentReads === 2) comments[0].body += "\nHuman revision.";
      return structuredClone(comments);
    }
    if ((method === "POST" && endpoint.endsWith("/comments")) || (method === "PATCH" && /^repos\/octo\/hello\/issues\/comments\/\d+$/.test(endpoint))) {
      const id = method === "POST" ? Math.max(0, ...comments.map(comment => comment.id)) + 1 : Number(endpoint.split("/").at(-1));
      const comment = { ...data, id, user: { login: context.bot }, html_url: `https://github.com/octo/hello/issues/12#issuecomment-${id}` };
      if (method === "POST") comments.push(comment);
      else comments.splice(comments.findIndex(comment => comment.id === id), 1, comment);
      if (options.lostCommentResponse) throw new Error("Response lost after write");
      return comment;
    }
    throw new Error(`Unexpected fake API call: ${method} ${endpoint}`);
  };
  return { api, calls, artifacts, comments, options };
}

function httpError(status) {
  return Object.assign(new Error(`Injected request failure (HTTP ${status})`), { status });
}

function oldArtifact(overrides = {}) {
  return {
    number: 1, type: "generic", name: "Duplicate analysis",
    body: mark(context, "duplicate_analysis", "Earlier result."),
    html_url: "https://github.com/octo/hello/issues/12/artifacts/1",
    creator: { login: context.bot }, updated_by_actor: { login: context.bot },
    updated_at: "2026-01-01T00:00:00Z", ...overrides,
  };
}

function oldComment({ number = 1, runId = "122", id = 2 } = {}) {
  return {
    id,
    user: { login: context.bot },
    body: mark(context, `comment-${runId}`, `Earlier comment.\n\n<!-- gh-aw-triage-reports:${JSON.stringify({ duplicate_analysis: number })} -->`),
    html_url: `https://github.com/octo/hello/issues/12#issuecomment-${id}`,
  };
}

test("publishReport", async t => {
  const cases = [
    {
      name: "preview writes each report and comment with no API calls",
      staged: true,
      check: ({ result, fake, files }) => {
        assert.equal(fake.calls.length, 0);
        assert.equal(result.staged, true);
        assert.match(files["comment.md"], /\[Reproduction log\]\(reproduction_log.md\)/);
        assert.equal(Object.keys(files).length, 6);
      },
    },
    {
      name: "four artifacts and one short linked comment",
      check: ({ result, fake, files }) => {
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.comments.length, 1);
        assert.deepEqual(result.failures, []);
        assert.match(files["comment.md"], /<summary>Supporting reports<\/summary>/);
        assert.doesNotMatch(files["comment.md"].split("<details>")[0], /duplicate|label|priority/i);
        assert.equal(fake.comments[0].body, files["comment.md"]);
      },
    },
    {
      name: "artifact-list 404 creates fresh reports and records warnings",
      options: { fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(404) },
      check: ({ result, fake, files }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.calls.filter(call => call.method === "POST" && call.endpoint.endsWith("/artifacts")).length, 4);
        assert.equal(result.warnings.length, 4);
        assert.deepEqual(JSON.parse(files["result.json"]).warnings, result.warnings);
        assert.ok(result.warnings.every(warning => warning.message.includes("HTTP 404")));
        assert.doesNotMatch(files["comment.md"], /HTTP 404|### Reproduction log/);
      },
    },
    {
      name: "artifact-list 404 creates new reports despite an earlier saved identity",
      options: {
        artifacts: [oldArtifact()],
        comments: [oldComment()],
        fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(404),
      },
      check: ({ result, fake, files }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(fake.artifacts.length, 5);
        assert.deepEqual(fake.artifacts[0], oldArtifact());
        assert.equal(fake.calls.filter(call => call.method === "PATCH").length, 0);
        assert.equal(result.artifacts.duplicate_analysis, "https://github.com/octo/hello/issues/12/artifacts/2");
        assert.match(files["comment.md"], /"duplicate_analysis":2/);
      },
    },
    ...[1, 2].map(read => ({
      name: `artifact ${read === 1 ? "detail" : "pre-update"} read 404 creates instead of patching`,
      options: {
        artifacts: [oldArtifact()],
        fail: (method, endpoint, data, calls) => method === "GET" && endpoint.endsWith("/artifacts/1") &&
          calls.filter(call => call.method === method && call.endpoint === endpoint).length === read && httpError(404),
      },
      check: ({ result, fake }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(result.warnings.length, 1);
        assert.equal(fake.artifacts.length, 5);
        assert.deepEqual(fake.artifacts[0], oldArtifact());
        assert.equal(fake.calls.filter(call => call.method === "PATCH").length, 0);
        assert.equal(result.artifacts.duplicate_analysis, "https://github.com/octo/hello/issues/12/artifacts/2");
      },
    })),
    ...[401, 403, 429, 500, 503].map(status => ({
      name: `artifact-read ${status} does not become an empty list`,
      options: { fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(status) },
      check: ({ result, fake }) => {
        assert.equal(result.failures.length, 4);
        assert.ok(result.failures.every(failure => failure.message.includes(`HTTP ${status}`)));
        assert.equal(result.warnings, undefined);
        assert.equal(fake.calls.filter(call => call.method === "POST" && call.endpoint.endsWith("/artifacts")).length, 0);
      },
    })),
    {
      name: "404 text without a structured HTTP status is still a failure",
      options: {
        fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") &&
          new Error("A payload mentioned HTTP 404"),
      },
      check: ({ result, fake }) => {
        assert.equal(result.failures.length, 4);
        assert.equal(result.warnings, undefined);
        assert.equal(fake.artifacts.length, 0);
      },
    },
    ...["repos/octo/hello/issues/12", "/comments?"].map(target => ({
      name: `${target.includes("comments") ? "comment" : "issue"} read 404 still blocks publication`,
      options: { fail: (method, endpoint) => method === "GET" && endpoint.includes(target) &&
        (target.includes("comments") || endpoint === target) && httpError(404) },
      error: /HTTP 404/,
    })),
    {
      name: "successive 404 uploads retain current identities when reads recover",
      options: { fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(404) },
      check: async ({ fake }) => {
        const firstBodies = fake.artifacts.map(artifact => artifact.body);
        for (const runId of ["124", "125"]) {
          const next = { ...context, runId };
          const result = await publishReport(parseReport(input(), next, identity), next, fake.api, () => {});
          assert.deepEqual(result.failures, []);
        }
        assert.equal(fake.artifacts.length, 12);
        assert.equal(fake.comments.length, 3);
        fake.options.fail = undefined;
        fake.comments.reverse();
        const next = { ...context, runId: "126" };
        const result = await publishReport(parseReport(input(), next, identity), next, fake.api, () => {});
        assert.deepEqual(result.failures, []);
        assert.equal(result.warnings, undefined);
        assert.equal(fake.artifacts.length, 12);
        assert.equal(fake.calls.filter(call => call.method === "PATCH").length, 4);
        assert.ok(fake.calls.filter(call => call.method === "PATCH").every(call => Number(call.endpoint.split("/").at(-1)) >= 9));
        assert.deepEqual(fake.artifacts.slice(0, 4).map(artifact => artifact.body), firstBodies);
        assert.equal(fake.comments.length, 4);
      },
    },
    {
      name: "an already published run does not duplicate reports after a read 404",
      options: { fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(404) },
      check: async ({ fake }) => {
        const writes = fake.calls.filter(call => call.method !== "GET").length;
        const report = parseReport(input(), context, identity);
        const reads = fake.calls.filter(call => call.method === "GET" && call.endpoint.includes("/artifacts")).length;
        const repeated = await publishReport(report, context, fake.api, () => {});
        assert.match(repeated.skipped, /already completed/);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.comments.length, 1);
        assert.equal(fake.calls.filter(call => call.method !== "GET").length, writes);
        assert.equal(fake.calls.filter(call => call.method === "GET" && call.endpoint.includes("/artifacts")).length, reads);
        fake.options.fail = undefined;
        const recovered = await publishReport(report, context, fake.api, () => {});
        assert.deepEqual(recovered.failures, []);
        assert.match(recovered.skipped, /already completed/);
        assert.equal(fake.calls.filter(call => call.method !== "GET").length, writes);
      },
    },
    {
      name: "a completed readable publication is not changed by a rerun",
      check: async ({ fake }) => {
        const original = structuredClone(fake.artifacts);
        const writes = fake.calls.filter(call => call.method !== "GET").length;
        const changed = input();
        decision(changed).investigation_notes = "Different evidence on a rerun.";
        const result = await publishReport(parseReport(changed, context, identity), context, fake.api, () => {});
        assert.match(result.skipped, /already completed/);
        assert.deepEqual(fake.artifacts, original);
        assert.equal(fake.calls.filter(call => call.method !== "GET").length, writes);
      },
    },
    {
      name: "an older run cannot replace a newer run's evidence",
      check: async ({ fake }) => {
        const newer = { ...context, runId: "124" };
        await publishReport(parseReport(input(), newer, identity), newer, fake.api, () => {});
        const original = structuredClone(fake.artifacts);
        const writes = fake.calls.filter(call => call.method !== "GET").length;
        await assert.rejects(publishReport(parseReport(input(), context, identity), context, fake.api, () => {}), /newer run already published/);
        assert.deepEqual(fake.artifacts, original);
        assert.equal(fake.calls.filter(call => call.method !== "GET").length, writes);
      },
    },
    {
      name: "duplicate keeps its single sentence and two reports",
      data: duplicateInput,
      check: ({ fake, files }) => {
        assert.equal(fake.artifacts.length, 2);
        assert.doesNotMatch(files["comment.md"], /<details>|Reproduction log|Investigation notes/);
      },
    },
    {
      name: "partial artifact failure falls back without repeating successful writes",
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log" },
      check: ({ result, fake, files }) => {
        assert.equal(fake.artifacts.length, 3);
        assert.equal(result.failures.length, 1);
        assert.match(files["comment.md"], /### Reproduction log/);
        assert.equal(fake.calls.filter(c => c.method === "POST" && c.data?.name === "Reproduction log").length, 1);
      },
    },
    {
      name: "a partial publication repairs its existing fallback comment",
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log" },
      check: async ({ fake }) => {
        fake.options.fail = undefined;
        const result = await publishReport(parseReport(input(), context, identity), context, fake.api, () => {});
        assert.deepEqual(result.failures, []);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.comments.length, 1);
        assert.doesNotMatch(fake.comments[0].body, /### Reproduction log/);
        assert.deepEqual(fake.calls.filter(call => call.method === "PATCH" && /\/comments\/\d+$/.test(call.endpoint)).map(call => call.endpoint),
          ["repos/octo/hello/issues/comments/1"]);
        assert.equal(fake.calls.filter(call => call.method === "PATCH" && call.endpoint.includes("/artifacts/")).length, 0);
      },
    },
    {
      name: "a partial duplicate publication can repair reports without expanding its comment",
      data: duplicateInput,
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Duplicate analysis" },
      check: async ({ fake }) => {
        fake.options.fail = undefined;
        const result = await publishReport(parseReport(duplicateInput(), context, identity), context, fake.api, () => {});
        assert.deepEqual(result.failures, []);
        assert.equal(fake.artifacts.length, 2);
        assert.equal(fake.comments.length, 1);
        assert.doesNotMatch(fake.comments[0].body, /<details>|###/);
      },
    },
    {
      name: "partial retries cannot replace the original input",
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log" },
      check: async ({ fake }) => {
        fake.options.fail = undefined;
        const changed = input();
        decision(changed).reproduction_log = "Different reproduction evidence.";
        const writes = fake.calls.filter(call => call.method !== "GET").length;
        await assert.rejects(publishReport(parseReport(changed, context, identity), context, fake.api, () => {}), /retry differs/);
        assert.equal(fake.calls.filter(call => call.method !== "GET").length, writes);
      },
    },
    {
      name: "human-edited fallback comments are not repaired",
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log" },
      check: async ({ fake }) => {
        fake.options.fail = undefined;
        fake.comments[0].body += "\nHuman revision.";
        const writes = fake.calls.filter(call => call.method !== "GET").length;
        await assert.rejects(publishReport(parseReport(input(), context, identity), context, fake.api, () => {}), /previous triage comment was edited/);
        assert.equal(fake.calls.filter(call => call.method !== "GET").length, writes);
      },
    },
    {
      name: "a fallback edited during repair is not overwritten",
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log" },
      check: async ({ fake }) => {
        fake.options.fail = undefined;
        fake.options.concurrentComment = true;
        await assert.rejects(publishReport(parseReport(input(), context, identity), context, fake.api, () => {}), /different comment already exists/);
        assert.match(fake.comments[0].body, /Human revision/);
        assert.equal(fake.calls.filter(call => call.method === "PATCH" && /\/comments\/\d+$/.test(call.endpoint)).length, 0);
      },
    },
    {
      name: "partial retries do not duplicate reports when reads are unavailable",
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log" },
      check: async ({ fake }) => {
        const writes = fake.calls.filter(call => call.method === "POST" && call.endpoint.endsWith("/artifacts")).length;
        fake.options.fail = (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(404);
        const result = await publishReport(parseReport(input(), context, identity), context, fake.api, () => {});
        assert.equal(result.failures.length, 4);
        assert.ok(result.failures.every(failure => failure.message.includes("refusing duplicate uploads")));
        assert.equal(fake.calls.filter(call => call.method === "POST" && call.endpoint.endsWith("/artifacts")).length, writes);
      },
    },
    {
      name: "a lost fallback-comment update response is reconciled without repeating the patch",
      options: {
        fail: (method, endpoint, data) => method === "POST" && data?.name === "Reproduction log",
        lostCommentResponse: true,
      },
      check: async ({ fake }) => {
        fake.options.fail = undefined;
        const result = await publishReport(parseReport(input(), context, identity), context, fake.api, () => {});
        assert.deepEqual(result.failures, []);
        assert.equal(fake.comments.length, 1);
        assert.equal(fake.calls.filter(call => call.method === "PATCH" && /\/comments\/\d+$/.test(call.endpoint)).length, 1);
      },
    },
    {
      name: "duplicate artifact failure does not expand the comment",
      data: duplicateInput,
      options: { fail: (method, endpoint, data) => method === "POST" && data?.name === "Duplicate analysis" },
      check: ({ result, files }) => {
        assert.equal(result.failures.length, 1);
        assert.doesNotMatch(files["comment.md"], /<details>|###/);
        assert.ok(files["duplicate_analysis.md"]);
      },
    },
    {
      name: "update own untouched report instead of duplicating",
      options: { artifacts: [oldArtifact()] },
      check: ({ fake }) => {
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.calls.filter(c => c.method === "PATCH").length, 1);
      },
    },
    {
      name: "human-authored report is not overwritten",
      options: { artifacts: [oldArtifact({ creator: { login: "human" } })] },
      check: ({ result, fake }) => {
        assert.equal(result.failures.length, 1);
        assert.equal(fake.calls.filter(c => c.method === "PATCH").length, 0);
        assert.equal(fake.artifacts[0].creator.login, "human");
      },
    },
    {
      name: "modified content fails its ownership checksum",
      options: { artifacts: [oldArtifact({ body: oldArtifact().body.replace("Earlier result.", "Human revision.") })] },
      check: ({ result, fake }) => {
        assert.match(result.failures[0].message, /edited/);
        assert.equal(fake.calls.filter(c => c.method === "PATCH").length, 0);
      },
    },
    {
      name: "human edits in version history are preserved",
      options: { artifacts: [oldArtifact()], historicalHuman: true },
      check: ({ result, fake }) => {
        assert.match(result.failures[0].message, /human edits/);
        assert.equal(fake.calls.filter(c => c.method === "PATCH").length, 0);
      },
    },
    {
      name: "concurrent edit is detected",
      options: { artifacts: [oldArtifact()], concurrent: true },
      check: ({ result, fake }) => {
        assert.match(result.failures[0].message, /changed while/);
        assert.equal(fake.calls.filter(c => c.method === "PATCH").length, 0);
      },
    },
    {
      name: "stable number preserves a report after its name and marker are changed",
      options: {
        artifacts: [oldArtifact({ name: "Human report", body: "Human content", updated_by_actor: { login: "human" } })],
        comments: [oldComment()],
      },
      check: ({ result, fake }) => {
        assert.match(result.failures[0].message, /belongs to a person or was edited/);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.artifacts[0].name, "Human report");
        assert.equal(fake.artifacts[0].body, "Human content");
        assert.equal(fake.calls.filter(c => c.method === "POST" && c.data?.name === "Duplicate analysis").length, 0);
      },
    },
    {
      name: "a deleted report is not recreated",
      options: { comments: [oldComment()] },
      check: ({ result, fake }) => {
        assert.match(result.failures[0].message, /removed or replaced/);
        assert.equal(fake.calls.filter(c => c.method === "POST" && c.data?.name === "Duplicate analysis").length, 0);
      },
    },
    {
      name: "an edited ownership record blocks writes",
      options: { comments: [{ ...oldComment(), body: oldComment().body.replace('analysis":1', 'analysis":2') }] },
      error: /previous triage comment was edited/,
    },
    {
      name: "lost artifact response is reconciled without a duplicate",
      options: { lostArtifactResponse: true },
      check: ({ result, fake }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.calls.filter(c => c.method === "POST" && c.endpoint.endsWith("/artifacts")).length, 4);
      },
    },
    {
      name: "lost artifact response and read 404 never retry the write",
      options: {
        lostArtifactResponse: true,
        fail: (method, endpoint) => method === "GET" && endpoint.includes("/artifacts?") && httpError(404),
      },
      check: ({ result, fake, files }) => {
        assert.equal(result.failures.length, 4);
        assert.ok(result.failures.every(failure => failure.message === "Response lost after write"));
        assert.equal(result.warnings.length, 4);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.calls.filter(call => call.method === "POST" && call.endpoint.endsWith("/artifacts")).length, 4);
        assert.match(files["comment.md"], /### Reproduction log/);
      },
    },
    ...[403, 404].map(status => ({
      name: `artifact-write ${status} remains a failure when reconciliation reads return 404`,
      options: {
        fail: (method, endpoint) => endpoint.includes("/artifacts") &&
          (method === "GET" ? httpError(404) : method === "POST" && httpError(status)),
      },
      check: ({ result, fake }) => {
        assert.equal(result.failures.length, 4);
        assert.ok(result.failures.every(failure => failure.message.includes(`HTTP ${status}`)));
        assert.equal(fake.artifacts.length, 0);
        assert.equal(fake.calls.filter(call => call.method === "POST" && call.endpoint.endsWith("/artifacts")).length, 4);
      },
    })),
    {
      name: "lost comment response is reconciled without repeating it",
      options: { lostCommentResponse: true },
      check: ({ result, fake }) => {
        assert.equal(fake.comments.length, 1);
        assert.equal(result.comment, fake.comments[0].html_url);
      },
    },
    {
      name: "closed issues are not changed",
      options: { closed: true }, error: /closed or locked/,
    },
    {
      name: "pull requests are not changed",
      options: { pr: true }, error: /not pull requests/,
    },
    {
      name: "synthetic PR output publishes unchanged without a repository lookup",
      data: fixtureInput,
      options: { private: false },
      check: ({ result, fake, files }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(fake.artifacts.length, 4);
        assert.equal(fake.comments.length, 1);
        assert.ok(files["comment.md"].includes(`$ gh pr create -f -r monalisa\n${fixtureURL}\n`));
        assert.ok(files["reproduction_log.md"].includes(`Captured stdout: \`${fixtureURL}\``));
        assert.equal(fake.calls.filter(call => call.endpoint.startsWith("repos/example.invalid/")).length, 0);
      },
    },
    {
      name: "synthetic shorthand and mixed-case owners do not need visibility lookups",
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).duplicate_analysis += "\nFixture: EXAMPLE.INVALID/hello#1.",
      check: ({ result, fake }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(fake.calls.filter(call => call.endpoint.startsWith("repos/EXAMPLE.INVALID/")).length, 0);
      },
    },
    {
      name: "public references alongside fixture output are still checked",
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).investigation_notes = "See [source](https://github.com/public/project/blob/main/file.go).",
      check: ({ result, fake }) => {
        assert.deepEqual(result.failures, []);
        assert.equal(fake.calls.filter(call => call.endpoint === "repos/public/project").length, 1);
      },
    },
    ...[
      "/../../hidden/project/issues/1",
      "/%2e%2e/%2e%2e/hidden/project/issues/1",
      "/..%2f..%2fhidden/project/issues/1",
      "/%252e%252e/%252e%252e/hidden/project/issues/1",
      "\\..\\..\\hidden\\project\\issues\\1",
      "/step)/../../../hidden/project/issues/1",
    ].map(suffix => ({
      name: `fixture URL cannot escape its owner through ${suffix}`,
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).investigation_notes = `See https://github.com/example.invalid/hello${suffix}`,
      error: /fixture URL/,
    })),
    {
      name: "ordinary fixture paths and encoded filename characters remain publishable",
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).investigation_notes = "See https://github.com/example.invalid/hello/blob/main/.github/file%20name.md.",
      check: ({ result }) => assert.deepEqual(result.failures, []),
    },
    {
      name: "adjacent fixture links cannot hide a canonical owner escape",
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).investigation_notes = "[a](https://github.com/example.invalid/hello)[b](https://github.com/example.invalid/hello/../../hidden/project/issues/1)",
      error: /fixture URL/,
    },
    ...["example.invalid-extra", "prefix-example.invalid", "missing"].map(owner => ({
      name: `${owner} is not the synthetic owner and a visibility 404 still blocks publication`,
      data: fixtureInput,
      options: { private: false, fail: (method, endpoint) =>
        method === "GET" && endpoint === `repos/${owner}/project` && httpError(404) },
      change: data => decision(data).investigation_notes = `See https://github.com/${owner}/project/issues/1.`,
      error: /HTTP 404/,
    })),
    {
      name: "private references in literal output are not exempted by the fixture namespace",
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).reproduction_log += "\n```text\nhttps://github.com/hidden/project/pull/1\n```",
      error: /non-public repository/,
    },
    {
      name: "private shorthand alongside fixture output still blocks publication",
      data: fixtureInput,
      options: { private: false },
      change: data => decision(data).investigation_notes = "See hidden/project#1.",
      error: /non-public repository/,
    },
    {
      name: "private references block public publication",
      options: { private: false },
      change: data => decision(data).investigation_notes = "See [evidence](https://github.com/hidden/project/issues/1).",
      error: /non-public repository/,
    },
    {
      name: "private shorthand references also block publication",
      options: { private: false },
      change: data => decision(data).investigation_notes = "See hidden/project#1.",
      error: /non-public repository/,
    },
    {
      name: "CLI transport pins host and passes content as stdin, never shell",
      transport: true,
      check: async () => {
        const observed = [];
        const api = ghClient((binary, args, opts) => {
          observed.push({ binary, args, opts });
          return "{}";
        });
        await api("POST", "repos/octo/hello/issues/12/artifacts", { body: "$(not executed)" });
        assert.equal(observed[0].binary, "gh");
        assert.deepEqual(observed[0].args.slice(0, 5), ["api", "--hostname", "github.com", "--method", "POST"]);
        assert.equal(observed[0].opts.input, '{"body":"$(not executed)"}');
        assert.equal(observed[0].opts.shell, undefined);
      },
    },
    {
      name: "transport errors never expose raw response contents",
      transport: true,
      check: async () => {
        const api = ghClient(() => { throw { stderr: "private payload (HTTP 403)", status: 1 }; });
        await assert.rejects(api("POST", "repos/octo/hello/issues/12/artifacts", {}), error =>
          error.status === 403 && error.message.includes("HTTP 403") && !error.message.includes("private payload"));
      },
    },
    {
      name: "CLI transport preserves read 404 without exposing the response",
      transport: true,
      check: async () => {
        const api = ghClient(() => { throw { stderr: "private payload (HTTP 404)", status: 1 }; });
        await assert.rejects(api("GET", "repos/octo/hello/issues/12/artifacts"), error =>
          error.status === 404 && error.message.includes("HTTP 404") && !error.message.includes("private payload"));
      },
    },
    {
      name: "a status in server text cannot replace the CLI status",
      transport: true,
      check: async () => {
        const api = ghClient(() => { throw { stderr: "private payload (HTTP 404) (HTTP 403)\n", status: 1 }; });
        await assert.rejects(api("GET", "repos/octo/hello/issues/12/artifacts"), error =>
          error.status === 403 && !error.message.includes("404") && !error.message.includes("private payload"));
      },
    },
  ];
  for (const entry of cases) await t.test(entry.name, async () => {
    if (entry.transport) return entry.check();
    const data = (entry.data ?? input)();
    entry.change?.(data);
    const report = parseReport(data, context, identity);
    const fake = fakeAPI(entry.options);
    const files = {};
    const operation = () => publishReport(report, { ...context, staged: entry.staged ?? false }, fake.api, (name, body) => {
      assert.equal(files[name], undefined, `File written twice: ${name}`);
      files[name] = body;
    });
    if (entry.error) {
      await assert.rejects(operation(), entry.error);
      assert.equal(fake.calls.filter(c => c.method !== "GET").length, 0);
    } else {
      const result = await operation();
      await entry.check?.({ result, fake, files });
    }
  });
});
