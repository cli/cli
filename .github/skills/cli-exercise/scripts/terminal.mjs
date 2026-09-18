import { createInterface } from "node:readline";
import { pathToFileURL, fileURLToPath } from "node:url";
import path from "node:path";

const pause = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

// The adapter only translates mechanical requests to the selected Tuistory API.
export async function serve(launchTerminal, input, output) {
  let session;
  let unsubscribe;
  let closing;
  let processGroup;
  let groupExited = false;
  let queue = Promise.resolve();
  let failed = false;
  const send = (value) => {
    const line = JSON.stringify(value) + "\n";
    if (Buffer.byteLength(line) > 64 * 1024 * 1024) throw new Error("adapter_message_limit");
    output.write(line);
  };
  const snapshot = (raw = "") => send({ type: "data", raw, data: session.getTerminalData() });
  const groupAlive = () => {
    if (groupExited) return false;
    try {
      process.kill(-processGroup, 0);
      return true;
    } catch (error) {
      // EPERM does not prove the group is gone, including while it is being reaped.
      if (error.code === "EPERM") return true;
      if (error.code !== "ESRCH") throw error;
      groupExited = true;
      send({ type: "process_group", pid: 0 });
      return false;
    }
  };
  const close = () => closing ??= (async () => {
    unsubscribe?.();
    if (!session) return;
    if (!processGroup) {
      session.close();
      throw new Error("unsupported_terminal_lifecycle");
    }
    // Session.close skips an exited leader; the pinned PTY handle still owns its group.
    if (groupAlive()) session.pty.kill();
    const deadline = Date.now() + 3500;
    while ((groupAlive() || !session.isDead) && Date.now() < deadline) await pause(10);
    if (groupAlive() || !session.isDead) throw new Error("adapter_cleanup_failed");
    session.close();
  })();
  const handle = async (request) => {
    if (!request || typeof request.id !== "string") throw new Error("invalid_adapter_request");
    if (request.op === "open") {
      if (session || closing) throw new Error("adapter_already_open");
      const options = request.launch;
      session = await launchTerminal({
        command: options.executable, args: options.args, cwd: options.cwd, env: options.env,
        cols: options.columns, rows: options.rows, idleDelayMs: 10, waitForData: false,
      });
      for (const name of ["subscribe", "getRawOutput", "getTerminalData", "writeRaw", "sendKey", "resize", "clickAt", "onExit", "close"]) {
        if (typeof session[name] !== "function") throw new Error("unsupported_terminal_api");
      }
      if (typeof session.isDead !== "boolean") throw new Error("unsupported_terminal_lifecycle");
      if (!Number.isSafeInteger(session.pty?.pid) || session.pty.pid <= 1 ||
          session.pty.pid === process.pid || typeof session.pty.kill !== "function") {
        throw new Error("unsupported_terminal_process_group");
      }
      processGroup = session.pty.pid;
      send({ type: "process_group", pid: processGroup });
      unsubscribe = session.subscribe((raw) => {
        if (!closing) {
          try { snapshot(raw); }
          catch { failed = true; send({ type: "error", code: "adapter_capture_failed" }); }
        }
      });
      snapshot(session.getRawOutput());
      session.onExit((info) => {
        if (!closing) {
          snapshot();
          send({ type: "exit", exitCode: info.signal ? null : info.exitCode, signal: info.signal || null });
        }
      });
      if (session.isDead) {
        const info = session.exitInfo;
        send({ type: "exit", exitCode: info?.signal ? null : info?.exitCode ?? null, signal: info?.signal || null });
      }
    } else if (request.op === "close") {
      await close();
    } else {
      if (!session || session.isDead || closing) throw new Error("adapter_target_closed");
      if (request.op === "reply") {
        if (typeof request.text !== "string") throw new Error("invalid_adapter_reply");
        session.writeRaw(request.text);
      } else if (request.op === "input") {
        const action = request.action;
        if (action.type === "text" && typeof action.text === "string") session.writeRaw(action.text);
        else if (action.type === "key") {
          if (typeof action.key === "string" && /^[A-Z]$/.test(action.key)) session.writeRaw(action.key);
          else session.sendKey(action.key);
        } else if (action.type === "resize") {
          session.resize({ cols: action.columns, rows: action.rows });
          snapshot();
        } else if (action.type === "click") await session.clickAt(action.x, action.y);
        else throw new Error("unsupported_adapter_input");
      } else throw new Error("unsupported_adapter_operation");
    }
    send({ type: "response", id: request.id, ok: true });
  };
  const lines = createInterface({ input, crlfDelay: Infinity });
  for await (const line of lines) {
    if (Buffer.byteLength(line) > 1024 * 1024) {
      failed = true;
      break;
    }
    queue = queue.then(async () => {
      let request;
      try {
        request = JSON.parse(line);
        await handle(request);
      } catch {
        failed = true;
        send({ type: "response", id: request?.id ?? "", ok: false, error: { code: "adapter_operation_failed" } });
      }
    });
    await queue;
    if (closing) break;
  }
  await close();
  return failed ? 1 : 0;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    if (process.argv.length !== 3) throw new Error("Use the Go owner to select the adapter library.");
    const entry = process.argv[2];
    if (!path.isAbsolute(entry)) throw new Error("The selected library must be absolute.");
    const library = await import(pathToFileURL(entry).href);
    process.exitCode = await serve(library.launchTerminal, process.stdin, process.stdout);
  } catch {
    process.stderr.write("The owned Tuistory adapter failed.\n");
    process.exitCode = 2;
  }
}
