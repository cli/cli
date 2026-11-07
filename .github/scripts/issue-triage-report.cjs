const { createHash } = require("node:crypto");
const { execFileSync } = require("node:child_process");
const fs = require("node:fs");
const path = require("node:path");

const reports = {
  duplicate_analysis: "Duplicate analysis",
  label_reasoning: "Label reasoning",
  reproduction_log: "Reproduction log",
  investigation_notes: "Investigation notes",
};
const openings = {
  reproduced: "Reproducible on",
  not_reproduced: "Not reproduced on",
  partial: "Partly reproduced on",
  blocked: "Unable to test:",
};
const digest = text => createHash("sha256").update(text).digest("hex");
const missingArtifact = Symbol("Artifact read returned HTTP 404");

function requireValue(condition, message) {
  if (!condition) throw new Error(message);
}

function linkLabels(markdown) {
  return markdown.replace(/\[([^\]]*)\]\([^)]+\)/g, "$1");
}

function wordCount(markdown) {
  return linkLabels(markdown)
    .replace(/<!--[\s\S]*?-->/g, "")
    .replace(/^```[^\n]*$/gm, "")
    .replace(/[*`_#|]/g, "")
    .trim().split(/\s+/).filter(Boolean).length;
}

function text(value, field, limit, sanitize) {
  requireValue(typeof value === "string", `${field} must be text`);
  requireValue(value.length <= limit, `${field} exceeds ${limit} characters`);
  requireValue(!/[\u0000-\u0008\u000b-\u001f\u007f\u202a-\u202e\u2066-\u2069]/u.test(value), `${field} contains control characters`);
  requireValue(!/\bghmock\b/i.test(linkLabels(value)), `${field} must describe gh, not the test backend`);
  requireValue(!value.includes("\u2014"), `${field} must use ordinary hyphens`);
  const clean = sanitize(value).trim();
  requireValue(clean.length <= limit, `${field} is too long after sanitization`);
  return clean;
}

function fencedBlocks(markdown) {
  const blocks = [];
  let current = null;
  for (const line of markdown.split("\n")) {
    if (/^```(?:console|sh|text)?$/.test(line)) {
      if (current === null) current = [];
      else {
        blocks.push(current);
        current = null;
      }
    } else if (current !== null) current.push(line);
    else requireValue(!/^\s*(?:```|~~~)/.test(line), "Use a plain three-backtick code fence");
  }
  requireValue(current === null, "Close the code fence");
  return blocks;
}

// All routing and resource identities come from the job, never the agent's text.
function parseReport(output, context, sanitize) {
  requireValue(output && Array.isArray(output.items), "Missing safe-output items");
  requireValue(/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(context.repo), "Invalid target repository");
  requireValue(Number.isSafeInteger(context.issue) && context.issue > 0, "Invalid target issue");
  requireValue(/^[1-9]\d*$/.test(context.runId), "Invalid workflow run");
  requireValue(/^[A-Za-z0-9_-]+$/.test(context.workflow), "Invalid workflow identity");
  const items = output.items.filter(item => item.type === "publish_triage");
  requireValue(items.length === 1, "Exactly one publish_triage output is required");
  requireValue(!output.items.some(item => ["add_comment", "apply_suspected_spam"].includes(item.type)), "Do not combine a triage report with another comment or spam action");
  const item = items[0];
  requireValue(["duplicate", "other", ...Object.keys(openings)].includes(item.outcome), "Invalid triage outcome");

  const labels = output.items.filter(item => item.type === "add_labels").flatMap(item => {
    requireValue(item.item_number === context.issue, "Label suggestions target a different issue");
    requireValue(!item.repo || item.repo === context.repo, "Label suggestions target a different repository");
    requireValue(Array.isArray(item.labels), "Label suggestions must be a list");
    return item.labels;
  });
  requireValue(labels.length <= 3, "At most three label suggestions are allowed");
  requireValue(new Set(labels.map(label => label.name)).size === labels.length, "Do not repeat label suggestions");
  for (const label of labels) {
    requireValue(label.suggest === true && ["HIGH", "MEDIUM", "LOW"].includes(label.confidence), "Labels must be suggestions with confidence");
    requireValue(/^[a-z][a-z0-9-]*$/.test(label.name) && label.name !== "needs-triage", "Invalid label suggestion");
    label.rationale = text(label.rationale, "label rationale", 600, sanitize);
    requireValue(label.rationale.length > 0, "Label suggestions need a reason");
  }

  const documents = {};
  for (const key of ["duplicate_analysis", "reproduction_log", "investigation_notes"]) {
    const body = text(item[key] ?? "", key, 10000, sanitize);
    if (body) documents[key] = body;
  }
  requireValue(documents.duplicate_analysis, "Include the duplicate search result or its limitation");
  if (labels.length) {
    documents.label_reasoning = labels.map(label =>
      `## \`${label.name}\`\n\n${label.rationale}\n\nConfidence: ${label.confidence.toLowerCase()}. Suggestion only.`
    ).join("\n\n");
  }

  let comment;
  if (item.outcome === "duplicate") {
    requireValue(labels.length === 1 && labels[0].name === "duplicate", "A duplicate can suggest only duplicate");
    requireValue(!documents.reproduction_log && !documents.investigation_notes, "Stop investigation after finding a duplicate");
    requireValue(/^[1-9]\d*$/.test(item.duplicate_issue ?? "") && Number(item.duplicate_issue) !== context.issue, "Choose another issue as the duplicate");
    const reason = text(item.duplicate_reason ?? "", "duplicate reason", 500, sanitize);
    requireValue(reason && !/\n|[.!?](?:\s|$)/.test(reason.replace(/`[^`]*`/g, "")), "Give one sentence fragment explaining the duplicate, without ending punctuation");
    requireValue(!item.comment, "The duplicate comment is generated from duplicate_issue and duplicate_reason");
    comment = `I believe this is a duplicate of [#${item.duplicate_issue}](https://github.com/cli/cli/issues/${item.duplicate_issue}) because ${reason}.`;
  } else {
    requireValue(!labels.some(label => label.name === "duplicate"), "Only a confirmed duplicate may suggest duplicate");
    comment = text(item.comment, "comment", 2500, sanitize);
    requireValue(comment, "Include a short comment");
    const blocks = fencedBlocks(comment);
    requireValue(blocks.length <= 1 && blocks.every(block => block.length <= 5), "Use at most one five-line command/output block");
    const prose = comment.replace(/```[\s\S]*?```/g, "").replace(/`[^`]*`/g, "");
    requireValue(!/<\/?[A-Za-z!]/.test(prose), "Do not supply HTML or details blocks in the visible comment");
    requireValue(!/\b(?:no (?:matching |clear )?duplicates?|duplicate (?:search|analysis|check)|suggest(?:ing|ed)? (?:the )?labels?|priority-[123]|off-schema|scaffold(?:ed|ing)?)\b/i.test(prose),
      "Keep label reasoning, negative duplicate discussion, and test machinery out of the visible comment");
    if (openings[item.outcome]) {
      requireValue(comment.startsWith(`**${openings[item.outcome]} `), "Start with the selected reproduction outcome in bold");
      requireValue(/^\*\*[^*\n]+\*\*(?:\n|$)/.test(comment), "Keep the complete result heading in bold");
      if (item.outcome !== "blocked") {
        requireValue(documents.reproduction_log, "Include the reproduction log");
        requireValue(blocks.length === 1 && /^\$ gh \S/.test(blocks[0][0] ?? ""), "Show the actual gh command");
        requireValue(blocks[0].length > 1 || /\b(?:no output|printed nothing|output:(?:\*\*)?\s*none)/i.test(comment), "Show captured output, or explicitly say the command printed nothing");
        requireValue(/\*\*Exit status:\*\* (?:`-?\d+`|not available)/.test(comment), "State the recorded exit status separately");
      }
    }
  }
  const runURL = `https://github.com/${context.repo}/actions/runs/${context.runId}`;
  const footer = `[Generated by issue triage](${runURL}).`;
  requireValue(wordCount(`${comment}\nSupporting reports\n${footer}`) <= 150, "The visible comment exceeds 150 words, including attribution");
  return { outcome: item.outcome, comment, documents, footer, runURL };
}

function marker(context, kind, body) {
  return `<!-- gh-aw-triage:${context.workflow}:${context.issue}:${kind}:${digest(body)} -->`;
}

function mark(context, kind, body) {
  return `${body}\n\n${marker(context, kind, body)}`;
}

function owned(context, kind, artifact) {
  if (artifact.creator?.login !== context.bot || artifact.updated_by_actor?.login !== context.bot) return false;
  const match = /^(.*)\n\n(<!-- gh-aw-triage:[^\n]+ -->)$/s.exec(artifact.body ?? "");
  return Boolean(match && match[2] === marker(context, kind, match[1]) && artifact.type === "generic" && artifact.name === reports[kind]);
}

function commentURL(context, comment) {
  requireValue(typeof comment.html_url === "string" &&
    comment.html_url.startsWith(`https://github.com/${context.repo}/issues/${context.issue}#issuecomment-`) &&
    /#issuecomment-[1-9]\d*$/.test(comment.html_url), "The API returned an unexpected comment URL");
  return comment.html_url;
}

function artifactURL(context, artifact) {
  requireValue(Number.isSafeInteger(artifact.number) && artifact.number > 0, "Invalid artifact number");
  const expected = `https://github.com/${context.repo}/issues/${context.issue}/artifacts/${artifact.number}`;
  requireValue(artifact.html_url === expected, "The API returned an unexpected artifact URL");
  return expected;
}

async function pages(api, endpoint) {
  const result = [];
  for (let page = 1; page <= 20; page++) {
    const items = await api("GET", `${endpoint}?per_page=100&page=${page}`);
    requireValue(Array.isArray(items), "Expected an API list");
    result.push(...items);
    if (items.length < 100) return result;
  }
  throw new Error("Too many existing reports to safely identify the workflow's output");
}

async function knownArtifacts(api, context) {
  const known = {};
  const latest = {};
  let alreadyPublished = false;
  let publication;
  let latestRun = 0n;
  const prefix = `<!-- gh-aw-triage:${context.workflow}:${context.issue}:comment-`;
  for (const comment of await pages(api, `repos/${context.repo}/issues/${context.issue}/comments`)) {
    if (comment.user?.login !== context.bot || !comment.body?.includes(prefix)) continue;
    const signed = /^(.*)\n\n(<!-- gh-aw-triage:[^\n]+ -->)$/s.exec(comment.body);
    const run = /:comment-([1-9]\d*):/.exec(signed?.[2] ?? "")?.[1];
    requireValue(signed && run && signed[2] === marker(context, `comment-${run}`, signed[1]),
      "A previous triage comment was edited; refusing to replace its report references");
    if (BigInt(run) > latestRun) latestRun = BigInt(run);
    if (run === context.runId) {
      requireValue(!publication, "More than one comment exists for this run");
      alreadyPublished = true;
      const state = /\n\n<!-- gh-aw-triage-publication:(\{[^\n]+\}) -->\n\n<!-- gh-aw-triage-reports:\{[^\n]+\} -->$/.exec(signed[1]);
      publication = { comment, state: state ? JSON.parse(state[1]) : null };
    }
    const registry = /\n\n<!-- gh-aw-triage-reports:(\{[^\n]+\}) -->$/.exec(signed[1]);
    if (!registry) continue;
    requireValue(Number.isSafeInteger(comment.id) && comment.id > 0, "Invalid saved comment identity");
    const entries = JSON.parse(registry[1]);
    for (const [kind, number] of Object.entries(entries)) {
      requireValue(Object.hasOwn(reports, kind) && Number.isSafeInteger(number) && number > 0, "Invalid saved report identity");
      if (!latest[kind] || comment.id > latest[kind]) {
        known[kind] = number;
        latest[kind] = comment.id;
      }
    }
  }
  return { known, alreadyPublished, publication, latestRun };
}

async function readArtifact(operation, onNotFound) {
  try {
    return await operation();
  } catch (error) {
    if (error.status !== 404) throw error;
    onNotFound();
    return missingArtifact;
  }
}

async function saveArtifact(api, context, kind, body, knownNumber, onNotFound) {
  const endpoint = `repos/${context.repo}/issues/${context.issue}/artifacts`;
  const listed = await readArtifact(() => pages(api, endpoint), onNotFound);
  const candidates = listed === missingArtifact ? [] : listed.filter(artifact => knownNumber
    ? artifact.number === knownNumber
    : artifact.name === reports[kind] ||
      artifact.body?.includes(`<!-- gh-aw-triage:${context.workflow}:${context.issue}:${kind}:`));
  requireValue(candidates.length <= 1, `${reports[kind]} has multiple existing copies; refusing to guess`);
  let previous = candidates[0];
  requireValue(listed === missingArtifact || !knownNumber || previous?.number === knownNumber,
    `${reports[kind]} was removed or replaced; leaving it unchanged`);
  if (previous) {
    const response = await readArtifact(() => api("GET", `${endpoint}/${previous.number}`), onNotFound);
    if (response === missingArtifact) {
      previous = undefined;
    } else {
      const live = response.artifact;
      requireValue(live && owned(context, kind, live), `${reports[kind]} belongs to a person or was edited; leaving it unchanged`);
      requireValue(Array.isArray(response.versions) && response.versions.every(version => version.actor?.login === context.bot),
        `${reports[kind]} has human edits in its history; leaving it unchanged`);
      if (live.body === body) return artifactURL(context, live);
      const refreshed = await readArtifact(() => api("GET", `${endpoint}/${previous.number}`), onNotFound);
      if (refreshed === missingArtifact) {
        previous = undefined;
      } else {
        requireValue(refreshed.artifact && owned(context, kind, refreshed.artifact) &&
          refreshed.artifact.body === live.body && refreshed.artifact.updated_at === live.updated_at,
          `${reports[kind]} changed while preparing the update; leaving it unchanged`);
      }
    }
  }
  const method = previous ? "PATCH" : "POST";
  const target = previous ? `${endpoint}/${previous.number}` : endpoint;
  const data = previous ? { name: reports[kind], body } : { type: "generic", name: reports[kind], body };
  try {
    return artifactURL(context, await api(method, target, data));
  } catch (error) {
    // A lost response can follow a successful write. Reconcile, never repeat it.
    const reconciled = await readArtifact(() => pages(api, endpoint), onNotFound);
    const found = (reconciled === missingArtifact ? [] : reconciled).filter(artifact =>
      artifact.name === reports[kind] && artifact.body === body && owned(context, kind, artifact)
    );
    if (found.length === 1) return artifactURL(context, found[0]);
    throw error;
  }
}

function renderComment(report, links, failed) {
  let body = report.comment;
  if (report.outcome !== "duplicate" && (Object.keys(links).length || failed.length)) {
    const contents = Object.entries(links).map(([key, url]) => `- [${reports[key]}](${url})`);
    for (const key of failed) contents.push(`### ${reports[key]}\n\n${report.documents[key]}`);
    body += `\n\n<details>\n<summary>Supporting reports</summary>\n\n${contents.join("\n\n")}\n\n</details>`;
  }
  return `${body}\n\n${report.footer}`;
}

async function saveComment(api, context, body, previous) {
  const endpoint = `repos/${context.repo}/issues/${context.issue}/comments`;
  const identity = `<!-- gh-aw-triage:${context.workflow}:${context.issue}:comment-${context.runId}:`;
  const matches = (await pages(api, endpoint)).filter(comment => comment.user?.login === context.bot && comment.body?.includes(identity));
  requireValue(matches.length <= 1, "More than one comment exists for this run");
  if (matches.length) {
    if (matches[0].body === body) return commentURL(context, matches[0]);
    requireValue(previous && matches[0].id === previous.id && matches[0].body === previous.body,
      "A different comment already exists for this run; refusing to overwrite it");
  } else {
    requireValue(!previous, "The previous triage comment was removed; refusing to replace it");
  }
  const method = matches.length ? "PATCH" : "POST";
  const target = matches.length ? `repos/${context.repo}/issues/comments/${matches[0].id}` : endpoint;
  try {
    return commentURL(context, await api(method, target, { body }));
  } catch (error) {
    const found = (await pages(api, endpoint)).filter(comment => comment.user?.login === context.bot && comment.body === body);
    if (found.length === 1) return commentURL(context, found[0]);
    throw error;
  }
}

function literalCodeAt(content, end) {
  const lines = content.slice(0, end).split("\n");
  // Keep containers literal instead of partially parsing their nested Markdown.
  if (lines.some(line => /^[ \t]*(?:>|[-+*][ \t]+|[0-9]+[.)][ \t]+)/.test(line))) return true;
  let fence = "";
  let inline = 0;
  for (const line of lines) {
    const marker = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line);
    if (fence) {
      if (marker && marker[1][0] === fence[0] && marker[1].length >= fence.length && !marker[2].trim()) fence = "";
      continue;
    }
    if (marker) {
      fence = marker[1];
      inline = 0;
      continue;
    }
    for (let index = 0; index < line.length; index++) {
      if (!inline && line[index] === "\\") {
        index++;
        continue;
      }
      if (line[index] !== "`") continue;
      let next = index + 1;
      while (line[next] === "`") next++;
      const length = next - index;
      if (!inline) inline = length;
      else if (inline === length) inline = 0;
      index = next - 1;
    }
  }
  return Boolean(fence || inline || /^(?: {4}|\t)/.test(lines.at(-1)));
}

function fixtureURLAt(content, start) {
  const suffix = content.slice(start);
  const quote = content[start - 1];
  const closing = { '"': '"', "'": "'", "`": "`", "<": ">" }[quote];
  const markdownLink = !literalCodeAt(content, start) && /\]\(\s*$/.test(content.slice(0, start));
  let parentheses = 0;
  let end = 0;
  for (; end < suffix.length; end++) {
    const character = suffix[end];
    if (/\s/.test(character) || character === closing) break;
    if ((closing || markdownLink) && character === "\\") {
      end++;
      continue;
    }
    if (markdownLink && character === "(") parentheses++;
    if (markdownLink && character === ")") {
      if (parentheses === 0) break;
      parentheses--;
    }
  }
  const value = suffix.slice(0, end);
  // Markdown escapes punctuation; those backslashes are not URL path separators.
  return markdownLink ? value.replace(/\\([\x21-\x2f\x3a-\x40\x5b-\x60\x7b-\x7e])/g, "$1") : value;
}

async function checkPublicReferences(api, context, report) {
  const target = await api("GET", `repos/${context.repo}`);
  requireValue(typeof target.private === "boolean", "Could not verify the target repository's visibility");
  if (target.private) return;
  const content = [report.comment, ...Object.values(report.documents)].join("\n");
  const repositories = new Set();
  // GitHub account names cannot contain periods, so this exact owner is fixture-only.
  const isFixture = repo => repo.split("/")[0].toLowerCase() === "example.invalid";
  for (const match of content.matchAll(/https:\/\/github\.com\/example\.invalid[/\\]/gi)) {
    // Markdown unescaping must not hide an owner escape in literal output.
    const literal = new URL(content.slice(match.index).match(/^\S+/)[0]);
    requireValue(isFixture(literal.pathname.slice(1)),
      "A fixture URL escapes the synthetic repository owner; refusing publication");
    const url = new URL(fixtureURLAt(content, match.index));
    requireValue(!/%(?:25|2f|5c)/i.test(url.pathname),
      "A fixture URL contains an ambiguous encoded path; refusing publication");
    requireValue(isFixture(url.pathname.slice(1)),
      "A fixture URL escapes the synthetic repository owner; refusing publication");
  }
  for (const match of content.matchAll(/https:\/\/github\.com\/([A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)(?=[/#?\s)]|$)/g)) {
    repositories.add(match[1]);
  }
  for (const match of content.matchAll(/\b([A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+)#[1-9]\d*/g)) {
    repositories.add(match[1]);
  }
  for (const repo of repositories) {
    if (repo === context.repo) continue;
    if (isFixture(repo)) continue;
    const referenced = await api("GET", `repos/${repo}`);
    requireValue(referenced.private === false, "A report references a non-public repository; refusing publication");
  }
}

// Preview and publication use the same rendered documents. Only API writes differ.
async function publishReport(report, context, api, writeFile) {
  const result = { staged: context.staged, artifacts: {}, failures: [], comment: null };
  const documents = {};
  for (const key of Object.keys(reports)) {
    if (!report.documents[key]) continue;
    documents[key] = mark(context, key, `${report.documents[key]}\n\n${report.footer}`);
    writeFile(`${key}.md`, documents[key]);
  }
  if (context.staged) {
    const links = Object.fromEntries(Object.keys(documents).map(key => [key, `${key}.md`]));
    const body = mark(context, `comment-${context.runId}`, renderComment(report, links, []));
    writeFile("comment.md", body);
    result.artifacts = links;
    result.comment = "comment.md";
    writeFile("result.json", JSON.stringify(result, null, 2));
    return result;
  }
  requireValue(/^[A-Za-z0-9-]+\[bot\]$/.test(context.bot ?? ""), "The publishing bot identity is missing");
  const issue = await api("GET", `repos/${context.repo}/issues/${context.issue}`);
  requireValue(!issue.pull_request, "Triage reports target issues, not pull requests");
  requireValue(issue.state === "open" && !issue.locked, "The issue is closed or locked; refusing publication");
  await checkPublicReferences(api, context, report);
  const { known, alreadyPublished, publication, latestRun } = await knownArtifacts(api, context);
  requireValue(BigInt(context.runId) >= latestRun, "A newer run already published triage; refusing stale report updates");
  const inputHash = digest(JSON.stringify({ comment: report.comment, documents }));
  if (publication) {
    requireValue(typeof publication.state?.complete === "boolean" &&
      /^[a-f0-9]{64}$/.test(publication.state?.input ?? ""),
    "This run has no publication state; start a new run instead of changing its reports");
    if (publication.state.complete) {
      result.artifacts = Object.fromEntries(Object.entries(known).map(([key, number]) =>
        [key, `https://github.com/${context.repo}/issues/${context.issue}/artifacts/${number}`]));
      result.comment = commentURL(context, publication.comment);
      result.skipped = "This run already completed publication; reports were left unchanged.";
      writeFile("comment.md", publication.comment.body);
      writeFile("result.json", JSON.stringify(result, null, 2));
      return result;
    }
    requireValue(publication.state.input === inputHash,
      "The retry differs from the partial publication; start a new run instead");
  }
  for (const [key, body] of Object.entries(documents)) {
    let warned = false;
    const onNotFound = () => {
      requireValue(!alreadyPublished, "Artifact read returned HTTP 404 after this run was published; refusing duplicate uploads");
      if (warned) return;
      warned = true;
      (result.warnings ??= []).push({ report: key, message: "Artifact read returned HTTP 404; treating the report as missing." });
    };
    try {
      result.artifacts[key] = await saveArtifact(api, context, key, body, known[key], onNotFound);
      known[key] = Number(result.artifacts[key].split("/").at(-1));
    } catch (error) {
      result.failures.push({ report: key, message: error.message });
    }
  }
  const state = `<!-- gh-aw-triage-publication:${JSON.stringify({ input: inputHash, complete: result.failures.length === 0 })} -->`;
  const registry = `<!-- gh-aw-triage-reports:${JSON.stringify(known)} -->`;
  const body = mark(context, `comment-${context.runId}`,
    `${renderComment(report, result.artifacts, result.failures.map(failure => failure.report))}\n\n${state}\n\n${registry}`);
  requireValue(body.length <= 60000, "Comment and fallback reports exceed the publication limit");
  writeFile("comment.md", body);
  try {
    result.comment = await saveComment(api, context, body, publication?.comment);
  } finally {
    writeFile("result.json", JSON.stringify(result, null, 2));
  }
  return result;
}

function ghClient(exec = execFileSync) {
  return async (method, endpoint, body) => {
    requireValue(endpoint.startsWith("repos/"), "Only repository API endpoints are permitted");
    const args = ["api", "--hostname", "github.com", "--method", method, endpoint,
      "-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: 2026-03-10"];
    if (body) args.push("--input", "-");
    let response;
    try {
      response = exec("gh", args, {
        input: body ? JSON.stringify(body) : undefined,
        encoding: "utf8", stdio: ["pipe", "pipe", "pipe"], timeout: 30000, maxBuffer: 8 * 1024 * 1024,
      });
    } catch (error) {
      const status = /\(HTTP (\d{3})\)\s*$/.exec(String(error.stderr))?.[1];
      const failure = new Error(`${method} request failed${status ? ` (HTTP ${status})` : ""}; no automatic write retry`);
      if (status) failure.status = Number(status);
      throw failure;
    }
    return JSON.parse(response);
  };
}

async function run({ env, sanitize, core }) {
  requireValue(env.GITHUB_SERVER_URL === "https://github.com", "Issue triage publishing is scoped to github.com");
  requireValue(env.GH_AW_DETECTION_CONCLUSION === "success", "The security check did not approve these outputs");
  const context = {
    repo: env.GITHUB_REPOSITORY,
    issue: Number(env.TRIAGE_ISSUE),
    runId: env.GITHUB_RUN_ID,
    workflow: env.GH_AW_WORKFLOW_ID,
    bot: env.TRIAGE_BOT_LOGIN,
    staged: env.GH_AW_SAFE_OUTPUTS_STAGED === "true",
  };
  const output = JSON.parse(fs.readFileSync(env.GH_AW_AGENT_OUTPUT, "utf8"));
  const report = parseReport(output, context, sanitize);
  const directory = path.join(env.RUNNER_TEMP, "triage-preview");
  fs.mkdirSync(directory, { recursive: false, mode: 0o700 });
  const writeFile = (name, data) => fs.writeFileSync(path.join(directory, name), data + "\n", { flag: "wx", mode: 0o600 });
  const result = await publishReport(report, context, ghClient(), writeFile);
  core.setOutput("status", result.staged || result.skipped ? "skipped" : result.failures.length ? "failure" : "success");
  if (!result.staged && result.comment) core.setOutput("url", result.comment);
  if (result.skipped) core.notice(result.skipped);
  for (const warning of result.warnings ?? []) core.warning(`${reports[warning.report]}: ${warning.message}`);
  for (const failure of result.failures) core.error(`${reports[failure.report]}: ${failure.message}`);
  if (result.failures.length) core.setFailed("Some report artifacts could not be published; their contents remain in the review files and comment fallback.");
}

module.exports = { parseReport, publishReport, ghClient, run, wordCount, mark, owned };
