// Extension: terminal-mockup
// Generate VSCode-style terminal screenshot mockups with dummy data
// for marketing materials. The canvas renders inside an iframe served
// by a loopback HTTP server; all editing, theming, and PNG export
// happen client-side in the iframe app.

import { createServer } from "node:http";
import { lstat, mkdir, readdir, readFile, unlink, writeFile } from "node:fs/promises";
import { dirname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";
import { homedir } from "node:os";
import { joinSession, createCanvas, CanvasError } from "@github/copilot-sdk/extension";

const __dirname = dirname(fileURLToPath(import.meta.url));
const ASSETS_DIR = join(__dirname, "assets");
const PROJECT_DIR = join(__dirname, "library");

const COPILOT_HOME = process.env.COPILOT_HOME || join(homedir(), ".copilot");
const USER_DIR = join(COPILOT_HOME, "extensions", "terminal-mockup", "artifacts");

const SCOPES = ["project", "user"];
const SCOPE_DIRS = { project: PROJECT_DIR, user: USER_DIR };
function isScope(s) { return s === "project" || s === "user"; }

const MIME = {
    ".html": "text/html; charset=utf-8",
    ".css": "text/css; charset=utf-8",
    ".js": "application/javascript; charset=utf-8",
    ".mjs": "application/javascript; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".svg": "image/svg+xml",
    ".png": "image/png",
    ".woff2": "font/woff2",
};

const instances = new Map();

function ensureInstanceState(instanceId) {
    let state = instances.get(instanceId);
    if (!state) {
        state = {
            content: "",
            options: {},
            sse: new Set(),
        };
        instances.set(instanceId, state);
    }
    return state;
}

function sendSse(state, res, payload) {
    if (res.destroyed || res.writableEnded) {
        state.sse.delete(res);
        return false;
    }
    try {
        res.write(`data: ${payload}\n\n`);
        return true;
    } catch {
        state.sse.delete(res);
        return false;
    }
}

function pushUpdate(instanceId) {
    const state = instances.get(instanceId);
    if (!state) return;
    const payload = JSON.stringify({ type: "state", content: state.content, options: state.options });
    for (const res of state.sse) sendSse(state, res, payload);
}

function broadcastLibraryChanged(payload = {}) {
    const event = JSON.stringify({ type: "library_changed", ...payload });
    for (const state of instances.values()) {
        for (const res of state.sse) sendSse(state, res, event);
    }
}

function slugify(name) {
    return String(name || "")
        .toLowerCase()
        .normalize("NFKD")
        .replace(/[^\w\s-]/g, "")
        .trim()
        .replace(/\s+/g, "-")
        .replace(/-+/g, "-")
        .slice(0, 80);
}

function isValidSlug(s) {
    return typeof s === "string" && /^[a-z0-9][a-z0-9-]{0,79}$/.test(s);
}

async function ensureDir(scope) {
    await mkdir(SCOPE_DIRS[scope], { recursive: true });
}

async function listScope(scope) {
    try {
        await ensureDir(scope);
        const entries = await readdir(SCOPE_DIRS[scope]);
        const out = [];
        for (const e of entries) {
            if (!e.endsWith(".json")) continue;
            const slug = e.slice(0, -5);
            try {
                const raw = await readFile(join(SCOPE_DIRS[scope], e), "utf8");
                const doc = JSON.parse(raw);
                out.push({ scope, slug, name: doc.name || slug, savedAt: doc.savedAt });
            } catch {
                out.push({ scope, slug, name: slug });
            }
        }
        return out;
    } catch {
        return [];
    }
}

async function listMockups() {
    const [projectItems, userItems] = await Promise.all([listScope("project"), listScope("user")]);
    const out = [...projectItems, ...userItems];
    out.sort((a, b) => {
        if (a.scope !== b.scope) return a.scope === "project" ? -1 : 1;
        return (a.name || "").localeCompare(b.name || "");
    });
    return out;
}

async function readMockup(slug, scope) {
    if (!isValidSlug(slug)) return null;
    const order = scope && isScope(scope) ? [scope] : SCOPES;
    for (const sc of order) {
        try {
            const raw = await readFile(join(SCOPE_DIRS[sc], `${slug}.json`), "utf8");
            const doc = JSON.parse(raw);
            return { ...doc, scope: sc, slug };
        } catch {}
    }
    return null;
}

async function refuseSymlink(path) {
    try {
        const stat = await lstat(path);
        if (stat.isSymbolicLink()) {
            throw new CanvasError("refused_symlink", `Refusing to operate on symlink: ${path}`);
        }
    } catch (err) {
        if (err && err.code === "ENOENT") return;
        throw err;
    }
}

async function writeMockup(slug, doc, scope) {
    if (!isValidSlug(slug)) throw new Error("invalid slug");
    if (!isScope(scope)) throw new Error("invalid scope");
    await ensureDir(scope);
    const target = join(SCOPE_DIRS[scope], `${slug}.json`);
    await refuseSymlink(target);
    await writeFile(target, JSON.stringify(doc, null, 2) + "\n", "utf8");
}

async function deleteMockup(slug, scope) {
    if (!isValidSlug(slug)) return false;
    if (!isScope(scope)) return false;
    const target = join(SCOPE_DIRS[scope], `${slug}.json`);
    try {
        await refuseSymlink(target);
        await unlink(target);
        return true;
    } catch {
        return false;
    }
}

async function readJsonBody(req) {
    return new Promise((resolve, reject) => {
        const chunks = [];
        let bytes = 0;
        req.on("data", (chunk) => {
            bytes += chunk.length;
            if (bytes > 5_000_000) {
                reject(new Error("body too large"));
                req.destroy();
                return;
            }
            chunks.push(chunk);
        });
        req.on("end", () => {
            if (bytes === 0) return resolve({});
            const body = Buffer.concat(chunks).toString("utf8");
            try { resolve(JSON.parse(body)); } catch (e) { reject(e); }
        });
        req.on("error", reject);
    });
}

async function serveStatic(req, res) {
    const url = new URL(req.url, "http://127.0.0.1");
    let path = decodeURIComponent(url.pathname);
    if (path === "/" || path === "") path = "/index.html";
    const safe = normalize(path).replace(/^[/\\]+/, "");
    const filePath = join(ASSETS_DIR, safe);
    if (!filePath.startsWith(ASSETS_DIR)) {
        res.statusCode = 403;
        res.end("Forbidden");
        return;
    }
    try {
        const data = await readFile(filePath);
        const ext = filePath.slice(filePath.lastIndexOf("."));
        res.setHeader("Content-Type", MIME[ext] || "application/octet-stream");
        res.setHeader("Cache-Control", "no-store");
        res.end(data);
    } catch (err) {
        res.statusCode = 404;
        res.end("Not found");
    }
}

function jsonResponse(res, status, body) {
    res.statusCode = status;
    res.setHeader("Content-Type", "application/json; charset=utf-8");
    res.setHeader("Cache-Control", "no-store");
    res.end(JSON.stringify(body));
}

async function handleMockupsApi(req, res, urlPath, instanceId) {
    // Routes:
    //   GET    /mockups                       → list merged
    //   POST   /mockups/<scope>               → create by name (server slugifies)
    //   GET    /mockups/<scope>/<slug>        → read specific scope
    //   PUT    /mockups/<scope>/<slug>        → write specific scope
    //   DELETE /mockups/<scope>/<slug>        → delete specific scope
    //   GET    /mockups/<slug>                → read (search project then user, back-compat)
    const method = req.method || "GET";
    const parts = urlPath.replace(/^\/mockups\/?/, "").split("/").filter(Boolean);

    try {
        if (parts.length === 0 && method === "GET") {
            return jsonResponse(res, 200, { items: await listMockups() });
        }
        if (parts.length === 1 && method === "POST" && isScope(parts[0])) {
            const scope = parts[0];
            const body = await readJsonBody(req);
            const slugified = slugify(body.name || body.slug || "");
            if (!isValidSlug(slugified)) return jsonResponse(res, 400, { error: "invalid_name" });
            const doc = {
                name: typeof body.name === "string" ? body.name : slugified,
                savedAt: new Date().toISOString(),
                content: typeof body.content === "string" ? body.content : "",
                options: body.options && typeof body.options === "object" ? body.options : {},
            };
            await writeMockup(slugified, doc, scope);
            return jsonResponse(res, 200, { ok: true, scope, slug: slugified, doc });
        }
        if (parts.length === 2 && isScope(parts[0])) {
            const [scope, slug] = parts;
            if (method === "GET") {
                const doc = await readMockup(slug, scope);
                if (!doc) return jsonResponse(res, 404, { error: "not_found" });
                return jsonResponse(res, 200, doc);
            }
            if (method === "PUT") {
                const body = await readJsonBody(req);
                if (!isValidSlug(slug)) return jsonResponse(res, 400, { error: "invalid_slug" });
                const doc = {
                    name: typeof body.name === "string" ? body.name : slug,
                    savedAt: new Date().toISOString(),
                    content: typeof body.content === "string" ? body.content : "",
                    options: body.options && typeof body.options === "object" ? body.options : {},
                };
                await writeMockup(slug, doc, scope);
                return jsonResponse(res, 200, { ok: true, scope, slug, doc });
            }
            if (method === "DELETE") {
                const ok = await deleteMockup(slug, scope);
                return jsonResponse(res, ok ? 200 : 404, { ok });
            }
        }
        if (parts.length === 1 && method === "GET") {
            // back-compat: GET /mockups/<slug>, search both scopes
            const doc = await readMockup(parts[0]);
            if (!doc) return jsonResponse(res, 404, { error: "not_found" });
            return jsonResponse(res, 200, doc);
        }
        return jsonResponse(res, 405, { error: "method_not_allowed" });
    } catch (err) {
        return jsonResponse(res, 500, { error: "server_error", message: String(err.message || err) });
    }
}

async function startServer(instanceId) {
    const state = ensureInstanceState(instanceId);
    let port = 0;
    const server = createServer((req, res) => {
        // Defense against DNS rebinding and same-port cross-origin loopback requests:
        // reject any request whose Host header does not match the loopback bound port,
        // or whose Origin (if present) is not loopback. Bound to 127.0.0.1, so the
        // only way to reach here with a foreign Host is a rebound DNS name.
        const host = req.headers.host || "";
        if (host !== `127.0.0.1:${port}` && host !== `localhost:${port}`) {
            res.statusCode = 403;
            res.end("Forbidden");
            return;
        }
        const origin = req.headers.origin;
        if (origin && !origin.startsWith("http://127.0.0.1:") && !origin.startsWith("http://localhost:")) {
            res.statusCode = 403;
            res.end("Forbidden");
            return;
        }
        const url = new URL(req.url, "http://127.0.0.1");
        if (url.pathname === "/state") {
            res.setHeader("Content-Type", "application/json; charset=utf-8");
            res.setHeader("Cache-Control", "no-store");
            res.end(JSON.stringify({ content: state.content, options: state.options }));
            return;
        }
        if (url.pathname === "/events") {
            res.statusCode = 200;
            res.setHeader("Content-Type", "text/event-stream");
            res.setHeader("Cache-Control", "no-store");
            res.setHeader("Connection", "keep-alive");
            state.sse.add(res);
            req.on("close", () => {
                state.sse.delete(res);
            });
            sendSse(state, res, JSON.stringify({ type: "state", content: state.content, options: state.options }));
            return;
        }
        if (url.pathname === "/mockups" || url.pathname.startsWith("/mockups/")) {
            handleMockupsApi(req, res, url.pathname, instanceId).catch((err) => {
                jsonResponse(res, 500, { error: "server_error", message: String(err.message || err) });
            });
            return;
        }
        serveStatic(req, res).catch(() => {
            res.statusCode = 500;
            res.end("Server error");
        });
    });
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    port = typeof address === "object" && address ? address.port : 0;
    return { server, url: `http://127.0.0.1:${port}/` };
}

await ensureDir("project").catch(() => {});

const session = await joinSession({
    canvases: [
        createCanvas({
            id: "terminal-mockup",
            displayName: "Terminal mockup",
            description: "Render dummy gh CLI output as a VSCode-styled terminal screenshot for marketing materials. Accepts raw ANSI or bracket markup. Supports a per-user saved-mockups library for managing multiple mockups in parallel.",
            inputSchema: {
                type: "object",
                properties: {
                    content: { type: "string", description: "Initial terminal content. Supports raw ANSI escape codes and bracket markup like [b]...[/b], [cyan]...[/cyan]." },
                    options: { type: "object", description: "Initial render options (chrome, backdrop, font, width)." },
                    loadSlug: { type: "string", description: "If set, load this saved mockup by slug on open." },
                    loadScope: { type: "string", enum: ["user", "project"], description: "Scope for loadSlug. If omitted, project is searched first then user." },
                },
            },
            actions: [
                {
                    name: "set_content",
                    description: "Replace the terminal content shown in the canvas. Supports ANSI escape codes and bracket markup.",
                    inputSchema: {
                        type: "object",
                        required: ["text"],
                        properties: {
                            text: { type: "string" },
                        },
                    },
                    handler: async (ctx) => {
                        const state = instances.get(ctx.instanceId);
                        if (!state) throw new CanvasError("not_open", "Canvas instance is not open");
                        const text = ctx.input && typeof ctx.input.text === "string" ? ctx.input.text : "";
                        state.content = text;
                        pushUpdate(ctx.instanceId);
                        return { ok: true, length: text.length };
                    },
                },
                {
                    name: "set_options",
                    description: "Adjust rendering options: chrome (none|macos), backdrop (none|solid|grid), font, fontSize, width, bodyGradient, autoStyle.",
                    inputSchema: {
                        type: "object",
                        properties: {
                            chrome: { type: "string", enum: ["none", "macos"] },
                            backdrop: { type: "string", enum: ["none", "solid", "grid"] },
                            font: { type: "string" },
                            fontSize: { type: "number" },
                            width: { type: "number" },
                            bodyGradient: { type: "boolean" },
                            autoStyle: { type: "boolean" },
                        },
                    },
                    handler: async (ctx) => {
                        const state = instances.get(ctx.instanceId);
                        if (!state) throw new CanvasError("not_open", "Canvas instance is not open");
                        state.options = { ...state.options, ...(ctx.input || {}) };
                        pushUpdate(ctx.instanceId);
                        return { ok: true, options: state.options };
                    },
                },
                {
                    name: "save_mockup",
                    description: "Save the current canvas content and options to a library. scope=\"user\" (default) writes to the per-user library; scope=\"project\" writes into the extension's committed library folder.",
                    inputSchema: {
                        type: "object",
                        required: ["name"],
                        properties: {
                            name: { type: "string", description: "Human-readable name. Slug is derived from this." },
                            slug: { type: "string", description: "Optional explicit slug. Must match [a-z0-9-]+." },
                            scope: { type: "string", enum: ["user", "project"], description: "Where to write. Defaults to user." },
                        },
                    },
                    handler: async (ctx) => {
                        const state = instances.get(ctx.instanceId);
                        if (!state) throw new CanvasError("not_open", "Canvas instance is not open");
                        const name = ctx.input && typeof ctx.input.name === "string" ? ctx.input.name : "";
                        const explicit = ctx.input && typeof ctx.input.slug === "string" ? ctx.input.slug : null;
                        const scope = isScope(ctx.input?.scope) ? ctx.input.scope : "user";
                        const slug = explicit && isValidSlug(explicit) ? explicit : slugify(name);
                        if (!isValidSlug(slug)) throw new CanvasError("invalid_name", "Name must contain at least one alphanumeric character");
                        const doc = {
                            name: name || slug,
                            savedAt: new Date().toISOString(),
                            content: state.content,
                            options: state.options,
                        };
                        try {
                            await writeMockup(slug, doc, scope);
                        } catch (err) {
                            throw new CanvasError("save_failed", String(err.message || err));
                        }
                        broadcastLibraryChanged({ action: "saved", scope, slug, name: doc.name });
                        return { ok: true, scope, slug, name: doc.name };
                    },
                },
                {
                    name: "load_mockup",
                    description: "Load a saved mockup by slug and apply its content + options to the canvas. If scope is omitted, the project library is searched first, then the user library.",
                    inputSchema: {
                        type: "object",
                        required: ["slug"],
                        properties: {
                            slug: { type: "string" },
                            scope: { type: "string", enum: ["user", "project"] },
                        },
                    },
                    handler: async (ctx) => {
                        const state = instances.get(ctx.instanceId);
                        if (!state) throw new CanvasError("not_open", "Canvas instance is not open");
                        const slug = ctx.input && typeof ctx.input.slug === "string" ? ctx.input.slug : "";
                        const scope = isScope(ctx.input?.scope) ? ctx.input.scope : undefined;
                        const doc = await readMockup(slug, scope);
                        if (!doc) throw new CanvasError("not_found", `No saved mockup with slug "${slug}"`);
                        state.content = typeof doc.content === "string" ? doc.content : "";
                        if (doc.options && typeof doc.options === "object") {
                            state.options = { ...state.options, ...doc.options };
                        }
                        pushUpdate(ctx.instanceId);
                        return { ok: true, scope: doc.scope, slug, name: doc.name };
                    },
                },
                {
                    name: "list_mockups",
                    description: "List all saved mockups from both the project (committed) library and the per-user library. Each item includes its scope.",
                    handler: async () => ({ items: await listMockups() }),
                },
                {
                    name: "delete_mockup",
                    description: "Delete a saved mockup by slug from the given scope.",
                    inputSchema: {
                        type: "object",
                        required: ["slug", "scope"],
                        properties: {
                            slug: { type: "string" },
                            scope: { type: "string", enum: ["user", "project"] },
                        },
                    },
                    handler: async (ctx) => {
                        const slug = ctx.input && typeof ctx.input.slug === "string" ? ctx.input.slug : "";
                        const scope = isScope(ctx.input?.scope) ? ctx.input.scope : "user";
                        const ok = await deleteMockup(slug, scope);
                        if (ok) broadcastLibraryChanged({ action: "deleted", scope, slug });
                        return { ok };
                    },
                },
                {
                    name: "batch_export",
                    description: "Tell the open iframe to download a PNG (or JPG) for each named saved mockup. All exports render with the iframe's current toolbar options (chrome, backdrop, font, etc.), NOT each mockup's saved options. Each item is either a bare slug (defaults to searching project then user) or a scoped string like \"project:my-slug\" / \"user:my-slug\". Filenames are `<slug><suffix>.<ext>`.",
                    inputSchema: {
                        type: "object",
                        required: ["slugs"],
                        properties: {
                            slugs: { type: "array", items: { type: "string" }, description: "Bare slug or \"<scope>:<slug>\"." },
                            suffix: { type: "string", description: "Suffix appended to slug before the extension (e.g. \"-no-frame\")." },
                            format: { type: "string", enum: ["png", "jpg"], description: "Optional. Defaults to whatever the toolbar has selected." },
                        },
                    },
                    handler: async (ctx) => {
                        const state = instances.get(ctx.instanceId);
                        if (!state) throw new CanvasError("not_open", "Canvas instance is not open");
                        const slugs = Array.isArray(ctx.input?.slugs) ? ctx.input.slugs.filter((s) => typeof s === "string") : [];
                        if (slugs.length === 0) throw new CanvasError("no_slugs", "Provide at least one slug to export");
                        if (state.sse.size === 0) {
                            throw new CanvasError("iframe_not_connected", "No iframe is connected to receive the export request. Open the canvas first.");
                        }
                        const suffix = typeof ctx.input?.suffix === "string" ? ctx.input.suffix : "";
                        const format = ctx.input?.format === "jpg" ? "jpg" : (ctx.input?.format === "png" ? "png" : null);
                        const payload = JSON.stringify({ type: "batch_export", slugs, suffix, format });
                        let delivered = 0;
                        for (const res of state.sse) {
                            if (sendSse(state, res, payload)) delivered++;
                        }
                        if (delivered === 0) {
                            throw new CanvasError("iframe_not_connected", "All iframe connections were stale; no exports were dispatched.");
                        }
                        return { ok: true, count: slugs.length, delivered };
                    },
                },
            ],
            open: async (ctx) => {
                const state = ensureInstanceState(ctx.instanceId);
                if (ctx.input && typeof ctx.input === "object") {
                    if (typeof ctx.input.loadSlug === "string") {
                        const loadScope = isScope(ctx.input.loadScope) ? ctx.input.loadScope : undefined;
                        const doc = await readMockup(ctx.input.loadSlug, loadScope);
                        if (doc) {
                            state.content = typeof doc.content === "string" ? doc.content : state.content;
                            if (doc.options && typeof doc.options === "object") {
                                state.options = { ...state.options, ...doc.options };
                            }
                        }
                    }
                    if (typeof ctx.input.content === "string") state.content = ctx.input.content;
                    if (ctx.input.options && typeof ctx.input.options === "object") {
                        state.options = { ...state.options, ...ctx.input.options };
                    }
                }
                let entry = state.server;
                if (!entry) {
                    entry = await startServer(ctx.instanceId);
                    state.server = entry;
                }
                pushUpdate(ctx.instanceId);
                return { title: "Terminal mockup", url: entry.url };
            },
            onClose: async (ctx) => {
                const state = instances.get(ctx.instanceId);
                if (!state) return;
                for (const res of state.sse) {
                    try { res.end(); } catch {}
                }
                state.sse.clear();
                if (state.server) {
                    state.server.server.closeAllConnections?.();
                    await new Promise((resolve) => state.server.server.close(() => resolve()));
                }
                instances.delete(ctx.instanceId);
            },
        }),
    ],
});
