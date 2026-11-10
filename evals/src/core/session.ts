import type { SessionConfig } from "@github/copilot-sdk";

/** Runs the actor before a successful tool result is released back to the agent. */
export function afterSuccessfulTool(config: SessionConfig, action: () => Promise<void>): SessionConfig {
  return {
    ...config,
    hooks: {
      ...config.hooks,
      async onPostToolUse(input, invocation) {
        const output = await config.hooks?.onPostToolUse?.(input, invocation);
        await action();
        return output;
      },
    },
  };
}
