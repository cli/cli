import { CopilotClient, type SessionConfig } from "@github/copilot-sdk";
import { chmod, copyFile, mkdir, writeFile } from "node:fs/promises";
import { delimiter, dirname, join } from "node:path";
import { isolatedEnvironment } from "./io.js";
import { writeGhWrapper } from "./observer.js";
import { afterSuccessfulTool } from "./session.js";

/**
 * Isolated Copilot client: no workspace config discovery, no extensions, and none of the runtime's
 * bundled skills, so skill-on and skill-off trials differ only by the skill under test.
 */
export class IsolatedClient extends CopilotClient {
  constructor(
    private readonly disabledBuiltins: string[],
    options: ConstructorParameters<typeof CopilotClient>[0],
    private readonly afterTool?: () => Promise<void>,
  ) {
    super(options);
  }

  override createSession(config: SessionConfig) {
    const isolated = {
      ...config,
      enableConfigDiscovery: false,
      requestExtensions: false,
      disabledSkills: [...new Set([...(config.disabledSkills ?? []), ...this.disabledBuiltins])],
    };
    return super.createSession(this.afterTool ? afterSuccessfulTool(isolated, this.afterTool) : isolated);
  }
}

/** Prepares an isolated agent home: gh (observed by default), its token, and a git credential helper. */
export async function prepareHome(
  home: string, config: { gh: string; token: string; modelToken: string; observe?: boolean },
): Promise<NodeJS.ProcessEnv> {
  for (const dir of ["bin", "tmp", ".config/gh", ".copilot"]) await mkdir(join(home, dir), { recursive: true });
  if (config.observe === false) {
    await copyFile(config.gh, join(home, "bin/gh"));
    await chmod(join(home, "bin/gh"), 0o700);
  } else {
    await copyFile(config.gh, join(home, "bin/gh-real"));
    await chmod(join(home, "bin/gh-real"), 0o700);
    await writeGhWrapper(join(home, "bin/gh"), join(home, "bin/gh-real"), join(home, ".commands.sock"));
  }
  const env = isolatedEnvironment(home, [
    join(home, "bin"), dirname(process.execPath), "/usr/bin", "/bin", "/usr/sbin", "/sbin",
  ].join(delimiter), config.modelToken);
  await writeFile(join(home, ".config/gh/hosts.yml"),
    `github.com:\n    oauth_token: ${JSON.stringify(config.token)}\n    git_protocol: https\n`, { mode: 0o600 });
  const credentialHelper = join(home, "bin/git-credential-fixture");
  await writeFile(credentialHelper, `#!${process.execPath}
let input = "";
for await (const chunk of process.stdin) input += chunk;
if (process.argv[2] === "get" && /^host=github\\.com$/m.test(input) && /^protocol=https$/m.test(input)) {
  process.stdout.write("username=x-access-token\\npassword=" + process.env.GH_EVAL_GIT_TOKEN + "\\n");
}
`, { mode: 0o700 });
  await writeFile(join(home, ".gitconfig"),
    `[user]\n\tname = Fixture author\n\temail = fixture@example.invalid\n[credential]\n\thelper =\n\thelper = ${JSON.stringify(`!${JSON.stringify(credentialHelper)}`)}\n`, { mode: 0o600 });
  env.GH_EVAL_GIT_TOKEN = config.token;
  return env;
}
