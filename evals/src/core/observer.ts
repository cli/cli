import type { Call } from "./calls.js";
import { record } from "./io.js";
import { createServer, type Server } from "node:net";
import { writeFile } from "node:fs/promises";

export function observe(calls: Call[], value: unknown, now: number): void {
  const event = record(value);
  if (event.type !== "tool.execution_start" && event.type !== "tool.execution_complete") return;
  const data = record(event.data);
  if (typeof data.toolCallId !== "string") throw new Error("SDK tool event is missing its call ID");
  const id = `tool:${data.toolCallId}`;
  if (event.type === "tool.execution_start") {
    if (calls.some(call => call.id === id)) throw new Error("Duplicate SDK tool call ID");
    calls.push({ id, kind: "tool", arguments: data.arguments, startedAt: now });
  } else {
    const call = calls.find(item => item.id === id);
    if (!call) throw new Error("SDK completed an unknown tool call");
    call.endedAt = now;
    call.success = data.success === true;
  }
}

export class CommandObserver {
  private readonly server: Server;
  error?: Error;

  constructor(
    private readonly calls: Call[],
    private readonly changed: (call: Call, phase: "start" | "output" | "end") => void,
  ) {
    this.server = createServer(socket => {
      let text = "";
      socket.setEncoding("utf8");
      socket.setTimeout(5_000, () => socket.destroy());
      socket.on("error", error => { this.error = error; });
      socket.on("data", chunk => {
        text += chunk;
        if (text.length > 2 * 1024 * 1024) {
          this.error = new Error("Command observer message exceeded 2 MiB");
          socket.destroy();
          return;
        }
        if (!text.endsWith("\n")) return;
        try {
          const message = record(JSON.parse(text));
          if (typeof message.id !== "string" || !/^gh:[0-9a-f-]{36}$/.test(message.id)) {
            throw new Error("Invalid command observer ID");
          }
          if (message.type === "start") {
            if (!Array.isArray(message.args) || !message.args.every(arg => typeof arg === "string")
              || this.calls.some(call => call.id === message.id)) throw new Error("Invalid observed command");
            this.calls.push({ id: message.id, kind: "gh", arguments: message.args, startedAt: Date.now(), output: [] });
          } else if (message.type === "output") {
            const call = this.calls.find(item => item.id === message.id);
            if (!call || call.kind !== "gh" || call.endedAt !== undefined || typeof message.text !== "string"
              || (message.stream !== "stdout" && message.stream !== "stderr")) throw new Error("Invalid command output");
            if (call.output.reduce((size, chunk) => size + Buffer.byteLength(chunk.text), 0)
              + Buffer.byteLength(message.text) > 2 * 1024 * 1024) throw new Error("Observed command output exceeded 2 MiB");
            call.output.push({ stream: message.stream, at: Date.now(), text: message.text });
          } else if (message.type === "end") {
            const call = this.calls.find(item => item.id === message.id);
            if (!call || call.kind !== "gh" || call.endedAt !== undefined) throw new Error("Invalid command completion");
            call.endedAt = Date.now();
            call.success = message.code === 0;
            call.exitCode = typeof message.code === "number" ? message.code : null;
          } else throw new Error("Invalid command observer message");
          const observed = this.calls.find(call => call.id === message.id);
          if (!observed) throw new Error("Missing observed command");
          this.changed(observed, message.type);
          socket.end("ok\n");
        } catch (error) {
          this.error = error instanceof Error ? error : new Error(String(error));
          socket.destroy();
        }
      });
    });
    this.server.on("error", error => { this.error = error; });
  }

  async listen(path: string): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      this.server.once("error", reject);
      this.server.listen(path, () => { this.server.removeListener("error", reject); resolve(); });
    });
  }

  async close(): Promise<void> {
    await new Promise<void>((resolve, reject) => this.server.close(error => error ? reject(error) : resolve()));
  }
}

export async function writeGhWrapper(path: string, binary: string, socket: string): Promise<void> {
  await writeFile(path, `#!${process.execPath}
const { spawn } = require("node:child_process");
const { connect } = require("node:net");
const { randomUUID } = require("node:crypto");
const { Transform } = require("node:stream");
const { pipeline } = require("node:stream/promises");
const { StringDecoder } = require("node:string_decoder");
const id = "gh:" + randomUUID();
function report(message) {
  return new Promise((resolve, reject) => {
    const socket = connect(${JSON.stringify(socket)});
    socket.setTimeout(5000, () => socket.destroy(new Error("Observer timed out")));
    socket.on("error", reject);
    let reply = "";
    socket.on("data", data => { reply += data; });
    socket.on("end", () => reply === "ok\\n" ? resolve() : reject(new Error("Observer rejected command")));
    socket.on("connect", () => socket.write(JSON.stringify({ ...message, id }) + "\\n"));
  });
}
function forward(source, destination, stream) {
  const decoder = new StringDecoder("utf8");
  return pipeline(source, new Transform({
    transform(chunk, _encoding, callback) {
      const text = decoder.write(chunk);
      (text ? report({ type: "output", stream, text }) : Promise.resolve())
        .then(() => callback(null, chunk), callback);
    },
    flush(callback) {
      const text = decoder.end();
      (text ? report({ type: "output", stream, text }) : Promise.resolve())
        .then(() => callback(), callback);
    }
  }), destination, { end: false });
}
(async () => {
  const args = process.argv.slice(2);
  const child = spawn(${JSON.stringify(binary)}, args, { stdio: ["inherit", "pipe", "pipe"], env: process.env });
  const completion = new Promise(resolve => {
    child.on("error", error => resolve({ error }));
    child.on("close", (code, signal) => resolve({ code, signal }));
  });
  const forwards = new Map();
  for (const signal of ["SIGINT", "SIGTERM", "SIGHUP"]) {
    const forward = () => { if (child.pid) child.kill(signal); };
    forwards.set(signal, forward);
    process.on(signal, forward);
  }
  await report({ type: "start", args });
  let forwardingError;
  const forwarding = Promise.allSettled([
    forward(child.stdout, process.stdout, "stdout"),
    forward(child.stderr, process.stderr, "stderr")
  ].map(pending => pending.catch(error => {
    forwardingError ??= error;
    child.kill("SIGTERM");
    throw error;
  })));
  const { code, signal, error } = await completion;
  await forwarding;
  if (error) {
    await report({ type: "end", code: 1 });
    throw error;
  }
  await report({ type: "end", code, signal });
  if (forwardingError) throw forwardingError;
  for (const [signal, forward] of forwards) process.removeListener(signal, forward);
  if (signal) process.kill(process.pid, signal);
  else process.exitCode = code;
})().catch(error => { console.error("gh command observer: " + error.message); process.exitCode = 1; });
`, { mode: 0o700 });
}

export function contamination(value: unknown, hiddenPaths: string[]): string | undefined {
  const text = JSON.stringify(value);
  return hiddenPaths.find(path => text.includes(path) || text.includes(path.replaceAll("/", "\\/")));
}

/** Returns enabled skills the runtime offered the agent beyond those the trial intends. */
export function unexpectedSkills(events: unknown[], expected: string[]): string[] {
  const offered = new Set<string>();
  for (const value of events) {
    if (typeof value !== "object" || value === null) continue;
    const event = value as Record<string, unknown>;
    if (event.type !== "session.skills_loaded") continue;
    const skills = (event.data as Record<string, unknown> | undefined)?.skills;
    if (!Array.isArray(skills)) continue;
    for (const skill of skills) {
      if (typeof skill === "object" && skill !== null && (skill as Record<string, unknown>).enabled !== false) {
        const name = (skill as Record<string, unknown>).name;
        if (typeof name === "string") offered.add(name);
      }
    }
  }
  return [...offered].filter(name => !expected.includes(name)).sort();
}
