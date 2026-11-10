const assert = require("node:assert/strict");
const { spawnSync } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { test } = require("node:test");

const script = path.join(__dirname, "issue-triage-ghmock.sh");
const fixture = `#!/bin/bash
if [ "$*" != version ]; then
  exit 2
fi
cat "$0.version"
exit "$(cat "$0.status")"
`;

function prepare(t, builds) {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "ghmock-build-inventory-"));
  t.after(() => fs.rmSync(tmp, { recursive: true, force: true }));
  const root = path.join(tmp, "root with spaces");
  fs.mkdirSync(path.join(root, "builds"), { recursive: true });
  for (const build of builds) {
    const dir = path.join(root, "builds", build.name);
    if (build.link) {
      fs.symlinkSync(build.link, dir);
      continue;
    }
    fs.mkdirSync(dir);
    fs.writeFileSync(path.join(dir, "gh"), fixture, { mode: build.executable === false ? 0o644 : 0o755 });
    fs.writeFileSync(path.join(dir, "gh.version"), build.version + "\n");
    fs.writeFileSync(path.join(dir, "gh.status"), String(build.status ?? 0));
  }
  const env = {
    PATH: "/usr/bin:/bin",
    HOME: tmp,
    TMPDIR: tmp,
    GH_CONFIG_DIR: path.join(tmp, "config"),
    GHMOCK_ROOT: root,
    LC_ALL: "C",
  };
  return { tmp, root, env };
}

test("record_builds", { skip: process.platform === "win32" }, async t => {
  const cases = [
    {
      name: "records source, release alias, and reported builds with observed versions",
      builds: [
        { name: "source", version: "gh version source-abcdefg" },
        { name: "2.103.3", version: "gh version 2.103.3 (2026-10-26)\nhttps://github.com/cli/cli/releases/tag/v2.103.3" },
        { name: "release", link: "2.103.3" },
        { name: "2.95.0", version: "gh version 2.95.0 (2026-06-17)" },
      ],
      expected: [
        ["2.103.3", "gh version 2.103.3 (2026-10-26)"],
        ["2.95.0", "gh version 2.95.0 (2026-06-17)"],
        ["release", "gh version 2.103.3 (2026-10-26)"],
        ["source", "gh version source-abcdefg"],
      ],
    },
    {
      name: "does not list a non-executable build",
      builds: [{ name: "source", version: "gh version source-abcdefg", executable: false }],
      expected: [],
    },
    {
      name: "keeps valid builds when another version check fails",
      builds: [
        { name: "source", version: "gh version source-abcdefg" },
        { name: "2.95.0", version: "gh version 2.95.0", status: 7 },
      ],
      expected: [["source", "gh version source-abcdefg"]],
      warning: true,
    },
    {
      name: "rejects successful output that is not a gh version",
      builds: [{ name: "source", version: "not a version" }],
      expected: [],
      warning: true,
    },
    {
      name: "rejects an empty version value",
      builds: [{ name: "source", version: "gh version " }],
      expected: [],
      warning: true,
    },
    { name: "records an explicit empty inventory", builds: [], expected: [] },
    {
      name: "replaces stale inventory instead of retaining unavailable builds",
      builds: [{ name: "source", version: "gh version source-abcdefg" }],
      expected: [["source", "gh version source-abcdefg"]],
      stale: true,
    },
    { name: "reports an inventory destination failure", builds: [], directory: true },
  ];
  for (const tc of cases) {
    await t.test(tc.name, t => {
      const { tmp, root, env } = prepare(t, tc.builds);
      const inventory = path.join(root, "builds.txt");
      if (tc.stale) fs.writeFileSync(inventory, "stale build\n");
      if (tc.directory) fs.mkdirSync(inventory);
      const result = spawnSync("/bin/bash", ["--noprofile", "--norc", "-c", 'source "$1"; record_builds', "_", script], {
        cwd: tmp, env, encoding: "utf8", timeout: 10000,
      });
      assert.ifError(result.error);
      assert.equal(result.status, tc.directory ? 1 : 0, result.stderr);
      assert.equal(result.stderr, "");
      assert.equal(result.stdout.includes("::warning::"), Boolean(tc.warning));
      assert.deepEqual(fs.readdirSync(root).sort(), ["builds", "builds.txt"]);
      if (tc.directory) {
        assert.deepEqual(fs.readdirSync(inventory), []);
        return;
      }
      const expected = tc.expected.length
        ? tc.expected.map(([name, version]) => `${path.join(root, "builds", name, "gh")}\n${version}\n\n`).join("")
        : "No verified gh builds are available.\n";
      assert.equal(fs.statSync(inventory).mode & 0o777, 0o644);
      assert.equal(fs.readFileSync(inventory, "utf8"), expected);
      assert.ok(result.stdout.endsWith(expected));
    });
  }
});

test("main", { skip: process.platform === "win32" }, async t => {
  const cases = [{ name: "keeps setup nonfatal and records no builds when ghmock cannot be built" }];
  for (const tc of cases) {
    await t.test(tc.name, t => {
      const { tmp, root, env } = prepare(t, []);
      const result = spawnSync("/bin/bash", ["--noprofile", "--norc", script], {
        cwd: tmp, env, encoding: "utf8", timeout: 10000,
      });
      assert.ifError(result.error);
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /::warning::Building ghmock failed/);
      assert.equal(fs.readFileSync(path.join(root, "builds.txt"), "utf8"), "No verified gh builds are available.\n");
    });
  }
});
