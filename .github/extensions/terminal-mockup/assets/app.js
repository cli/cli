// App glue: wires editor + toolbar to the renderer, listens for state pushes
// from the extension over SSE, and handles PNG export via html2canvas.

import { renderToDom } from "./ansi.js";

const $ = (sel) => document.querySelector(sel);

const editor = $("#editor");
const terminal = $("#terminal");
const windowEl = $("#window");
const mockup = $("#mockup");
const fontSel = $("#ctl-font");
const fontSize = $("#ctl-fontsize");
const fontSizeOut = $("#ctl-fontsize-out");
const widthIn = $("#ctl-width");
const widthOut = $("#ctl-width-out");
const chromeSel = $("#ctl-chrome");
const backdropSel = $("#ctl-backdrop");
const bodyGradCb = $("#ctl-bodygrad");
const autoStyleCb = $("#ctl-autostyle");
const downloadBtn = $("#btn-download");
const savedSel = $("#ctl-saved");
const saveAsBtn = $("#btn-save");
const saveAsProjectBtn = $("#btn-save-project");
const saveOverwriteBtn = $("#btn-save-overwrite");
const deleteBtn = $("#btn-delete");
const toast = $("#toast");
const formatSel = $("#ctl-format");

let state = {
    content: "",
    options: {
        font: "menlo",
        fontSize: 14,
        width: 800,
        chrome: "none",
        backdrop: "none",
        bodyGradient: false,
        autoStyle: true,
    },
};

function applyState() {
    editor.value = state.content;
    fontSel.value = state.options.font;
    fontSize.value = String(state.options.fontSize);
    fontSizeOut.textContent = `${state.options.fontSize}px`;
    widthIn.value = String(state.options.width);
    widthOut.textContent = `${state.options.width}px`;
    chromeSel.value = state.options.chrome;
    backdropSel.value = state.options.backdrop;
    bodyGradCb.checked = !!state.options.bodyGradient;
    autoStyleCb.checked = !!state.options.autoStyle;
    rerender();
}

function rerender() {
    // Apply visual options
    windowEl.dataset.font = state.options.font;
    windowEl.classList.toggle("has-chrome", state.options.chrome === "macos");
    windowEl.classList.toggle("no-chrome", state.options.chrome === "none");
    windowEl.classList.toggle("body-gradient", !!state.options.bodyGradient);
    windowEl.style.setProperty("--mockup-width", `${state.options.width}px`);
    terminal.style.setProperty("--term-fontsize", `${state.options.fontSize}px`);

    mockup.classList.remove("backdrop-grid", "backdrop-solid", "backdrop-none");
    mockup.classList.add(`backdrop-${state.options.backdrop}`);

    renderToDom(terminal, state.content, { autoStyle: state.options.autoStyle });
}

// Initial load: pull server-side state set via canvas open input or set_content action.
async function init() {
    try {
        const res = await fetch("/state", { cache: "no-store" });
        if (res.ok) {
            const remote = await res.json();
            if (remote && typeof remote.content === "string" && remote.content.trim().length > 0) {
                state.content = remote.content;
            }
            if (remote && remote.options && typeof remote.options === "object") {
                state.options = { ...state.options, ...remote.options };
            }
        }
    } catch {
        // ignore; fall back to defaults
    }
    applyState();
    connectSse();
}

function connectSse() {
    let es;
    const open = () => {
        es = new EventSource("/events");
        es.onmessage = (evt) => {
            try {
                const data = JSON.parse(evt.data);
                if (data && data.type === "library_changed") {
                    if (data.action === "saved" && typeof data.slug === "string" && (data.scope === "project" || data.scope === "user")) {
                        loadedSlug = data.slug;
                        loadedScope = data.scope;
                        loadedName = data.name || data.slug;
                    } else if (data.action === "deleted" && typeof data.slug === "string" && data.slug === loadedSlug && data.scope === loadedScope) {
                        loadedSlug = null;
                        loadedScope = null;
                        loadedName = null;
                    }
                    refreshLibrary().then(() => updateLoadedAffordances());
                    return;
                }
                if (data && data.type === "batch_export") {
                    runBatchExport(data).catch((err) => showToast(`Batch export failed: ${err.message}`));
                    return;
                }
                let changed = false;
                if (typeof data.content === "string" && data.content !== state.content) {
                    state.content = data.content;
                    changed = true;
                }
                if (data.options && typeof data.options === "object") {
                    const next = { ...state.options, ...data.options };
                    if (JSON.stringify(next) !== JSON.stringify(state.options)) {
                        state.options = next;
                        changed = true;
                    }
                }
                if (changed) applyState();
            } catch {}
        };
        es.onerror = () => {
            es.close();
            setTimeout(open, 1500);
        };
    };
    open();
}

// Event wiring
editor.addEventListener("input", () => {
    state.content = editor.value;
    rerender();
});

fontSel.addEventListener("change", () => {
    state.options.font = fontSel.value;
    rerender();
});

fontSize.addEventListener("input", () => {
    state.options.fontSize = Number(fontSize.value);
    fontSizeOut.textContent = `${fontSize.value}px`;
    rerender();
});

widthIn.addEventListener("input", () => {
    state.options.width = Number(widthIn.value);
    widthOut.textContent = `${widthIn.value}px`;
    rerender();
});

chromeSel.addEventListener("change", () => {
    state.options.chrome = chromeSel.value;
    rerender();
});

backdropSel.addEventListener("change", () => {
    state.options.backdrop = backdropSel.value;
    rerender();
});

bodyGradCb.addEventListener("change", () => {
    state.options.bodyGradient = bodyGradCb.checked;
    rerender();
});

autoStyleCb.addEventListener("change", () => {
    state.options.autoStyle = autoStyleCb.checked;
    rerender();
});

// Saved-mockups library. Two scopes:
//   project: .github/extensions/terminal-mockup/library/ (committed, shared)
//   user:    ~/.copilot/extensions/terminal-mockup/artifacts/ (per-user)
let loadedSlug = null;
let loadedScope = null;
let loadedName = null;

function scopedId(scope, slug) { return `${scope}:${slug}`; }
function parseScopedId(value) {
    if (!value) return null;
    const i = value.indexOf(":");
    if (i < 1) return null;
    const scope = value.slice(0, i);
    const slug = value.slice(i + 1);
    if (scope !== "project" && scope !== "user") return null;
    if (!slug) return null;
    return { scope, slug };
}
function scopeLabel(scope) { return scope === "project" ? "Project" : "Local"; }

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

function updateLoadedAffordances() {
    const has = !!loadedSlug;
    saveOverwriteBtn.disabled = !has;
    deleteBtn.disabled = !has;
    if (has) {
        const label = loadedName || loadedSlug;
        const scopeTag = loadedScope === "project" ? " (Project)" : " (Local)";
        saveOverwriteBtn.textContent = `Save "${label}"${scopeTag}`;
        deleteBtn.textContent = loadedScope === "project" ? "Delete from project" : "Delete";
    } else {
        saveOverwriteBtn.textContent = "Save";
        deleteBtn.textContent = "Delete";
    }
}

async function refreshLibrary() {
    try {
        const res = await fetch("/mockups", { cache: "no-store" });
        if (!res.ok) return;
        const data = await res.json();
        const items = Array.isArray(data.items) ? data.items : [];
        savedSel.innerHTML = '<option value="">Select a mockup</option>';
        const groups = { project: [], user: [] };
        for (const it of items) {
            if (it && (it.scope === "project" || it.scope === "user")) groups[it.scope].push(it);
        }
        for (const scope of ["project", "user"]) {
            if (groups[scope].length === 0) continue;
            const og = document.createElement("optgroup");
            og.label = scopeLabel(scope);
            for (const it of groups[scope]) {
                const opt = document.createElement("option");
                opt.value = scopedId(scope, it.slug);
                opt.textContent = it.name || it.slug;
                og.appendChild(opt);
            }
            savedSel.appendChild(og);
        }
        if (loadedSlug && loadedScope && items.some((i) => i.scope === loadedScope && i.slug === loadedSlug)) {
            savedSel.value = scopedId(loadedScope, loadedSlug);
        }
    } catch (e) {
        // ignore; library just stays empty
    }
}

async function loadMockup(scope, slug) {
    if (!slug || !scope) {
        loadedSlug = null;
        loadedScope = null;
        loadedName = null;
        updateLoadedAffordances();
        return;
    }
    try {
        const res = await fetch(`/mockups/${encodeURIComponent(scope)}/${encodeURIComponent(slug)}`, { cache: "no-store" });
        if (!res.ok) throw new Error(`load failed: ${res.status}`);
        const doc = await res.json();
        state.content = typeof doc.content === "string" ? doc.content : "";
        state.options = { ...state.options, ...(doc.options || {}) };
        loadedSlug = slug;
        loadedScope = scope;
        loadedName = doc.name || slug;
        applyState();
        updateLoadedAffordances();
        showToast(`Loaded "${loadedName}" (${scopeLabel(scope)})`);
    } catch (e) {
        showToast(`Load failed: ${e.message}`);
    }
}

async function saveMockup(scope, name, slug) {
    const body = {
        name: name || slug,
        content: state.content,
        options: state.options,
    };
    const url = slug
        ? `/mockups/${encodeURIComponent(scope)}/${encodeURIComponent(slug)}`
        : `/mockups/${encodeURIComponent(scope)}`;
    if (!slug) body.name = name;
    const res = await fetch(url, {
        method: slug ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
    if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.error || `save failed: ${res.status}`);
    }
    return await res.json();
}

savedSel.addEventListener("change", () => {
    const parsed = parseScopedId(savedSel.value);
    if (parsed) loadMockup(parsed.scope, parsed.slug);
    else {
        loadedSlug = null;
        loadedScope = null;
        loadedName = null;
        updateLoadedAffordances();
    }
});

function promptSaveName(defaultValue, scope) {
    return new Promise((resolve) => {
        const dialog = document.getElementById("save-dialog");
        const input = document.getElementById("save-name");
        const cancel = document.getElementById("save-cancel");
        const form = document.getElementById("save-form");
        const heading = document.getElementById("save-heading");
        if (!dialog || typeof dialog.showModal !== "function") {
            const v = window.prompt(`Save mockup to ${scopeLabel(scope)} library as:`, defaultValue || "");
            resolve(v && v.trim() ? v.trim() : null);
            return;
        }
        if (heading) heading.textContent = scope === "project" ? "Save to project library" : "Save to local library";
        input.value = defaultValue || "";
        let settled = false;
        const settle = (value) => {
            if (settled) return;
            settled = true;
            form.removeEventListener("submit", onSubmit);
            cancel.removeEventListener("click", onCancel);
            dialog.removeEventListener("close", onClose);
            resolve(value);
        };
        const onSubmit = (e) => {
            e.preventDefault();
            const value = (input.value || "").trim();
            settle(value || null);
            dialog.close(value ? "ok" : "");
        };
        const onCancel = () => {
            settle(null);
            dialog.close("");
        };
        const onClose = () => settle(null);
        form.addEventListener("submit", onSubmit);
        cancel.addEventListener("click", onCancel);
        dialog.addEventListener("close", onClose);
        dialog.showModal();
        setTimeout(() => input.focus(), 0);
        input.select();
    });
}

async function saveAs(scope, button) {
    const name = await promptSaveName(loadedName || "", scope);
    if (!name) return;
    const slug = slugify(name);
    if (!slug) {
        showToast("Name needs at least one alphanumeric character");
        return;
    }
    button.disabled = true;
    try {
        const result = await saveMockup(scope, name, slug);
        loadedScope = result.scope || scope;
        loadedSlug = result.slug;
        loadedName = result.doc?.name || name;
        await refreshLibrary();
        savedSel.value = scopedId(loadedScope, loadedSlug);
        updateLoadedAffordances();
        showToast(`Saved "${loadedName}" to ${scopeLabel(loadedScope)} library`);
    } catch (e) {
        showToast(`Save failed: ${e.message}`);
    } finally {
        button.disabled = false;
    }
}

saveAsBtn.addEventListener("click", () => saveAs("user", saveAsBtn));
if (saveAsProjectBtn) {
    saveAsProjectBtn.addEventListener("click", () => saveAs("project", saveAsProjectBtn));
}

saveOverwriteBtn.addEventListener("click", async () => {
    if (!loadedSlug || !loadedScope) return;
    saveOverwriteBtn.disabled = true;
    try {
        await saveMockup(loadedScope, loadedName || loadedSlug, loadedSlug);
        showToast(`Saved "${loadedName || loadedSlug}" to ${scopeLabel(loadedScope)}`);
    } catch (e) {
        showToast(`Save failed: ${e.message}`);
    } finally {
        updateLoadedAffordances();
    }
});

deleteBtn.addEventListener("click", async () => {
    if (!loadedSlug || !loadedScope) return;
    const scopeMsg = loadedScope === "project" ? " from the project library (will show as a deleted file in git)" : "";
    if (!confirm(`Delete "${loadedName || loadedSlug}"${scopeMsg}?`)) return;
    try {
        const res = await fetch(`/mockups/${encodeURIComponent(loadedScope)}/${encodeURIComponent(loadedSlug)}`, { method: "DELETE" });
        if (!res.ok) throw new Error(`delete failed: ${res.status}`);
        showToast(`Deleted "${loadedName || loadedSlug}"`);
        loadedSlug = null;
        loadedScope = null;
        loadedName = null;
        await refreshLibrary();
        updateLoadedAffordances();
    } catch (e) {
        showToast(`Delete failed: ${e.message}`);
    }
});

// Refresh library on init
refreshLibrary();

// Export
function currentFormat() {
    const v = (formatSel && formatSel.value) || "png";
    if (v === "jpg" || v === "jpeg") {
        return { ext: "jpg", mime: "image/jpeg", quality: 0.92, label: "JPG", background: "#04060c" };
    }
    return { ext: "png", mime: "image/png", quality: undefined, label: "PNG", background: null };
}

async function renderToCanvas(background) {
    // Wait one tick so fonts settle if user just changed them
    await document.fonts.ready;
    const canvas = await html2canvas(mockup, {
        backgroundColor: background ?? null,
        scale: 3,
        useCORS: true,
        logging: false,
    });
    return canvas;
}

function updateExportLabels() {
    const fmt = currentFormat();
    downloadBtn.textContent = `Download ${fmt.label}`;
}
if (formatSel) {
    formatSel.addEventListener("change", updateExportLabels);
    updateExportLabels();
}

function showToast(msg) {
    toast.textContent = msg;
    toast.hidden = false;
    clearTimeout(showToast._t);
    showToast._t = setTimeout(() => { toast.hidden = true; }, 2200);
}

downloadBtn.addEventListener("click", async () => {
    downloadBtn.disabled = true;
    try {
        const fmt = currentFormat();
        const canvas = await renderToCanvas(fmt.background);
        const blob = await new Promise((resolve) => canvas.toBlob(resolve, fmt.mime, fmt.quality));
        if (!blob) throw new Error(`Could not encode ${fmt.label}`);
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `${loadedSlug || "gh-terminal-mockup"}.${fmt.ext}`;
        document.body.appendChild(a);
        a.click();
        a.remove();
        setTimeout(() => URL.revokeObjectURL(url), 1000);
        showToast(`Saved ${fmt.label}`);
    } catch (e) {
        showToast(`Export failed: ${e.message}`);
    } finally {
        downloadBtn.disabled = false;
    }
});

async function runBatchExport({ slugs, suffix, format }) {
    if (!Array.isArray(slugs) || slugs.length === 0) return;
    const fmtOverride = format === "jpg" ? { ext: "jpg", mime: "image/jpeg", quality: 0.92, label: "JPG", background: "#04060c" }
        : format === "png" ? { ext: "png", mime: "image/png", quality: undefined, label: "PNG", background: null }
        : null;
    const savedContent = state.content;
    const savedSlugRef = loadedSlug;
    const savedScopeRef = loadedScope;
    const savedNameRef = loadedName;
    downloadBtn.disabled = true;
    try {
        for (const entry of slugs) {
            const parsed = parseScopedId(entry);
            const slug = parsed ? parsed.slug : entry;
            const url = parsed
                ? `/mockups/${encodeURIComponent(parsed.scope)}/${encodeURIComponent(parsed.slug)}`
                : `/mockups/${encodeURIComponent(slug)}`;
            try {
                const res = await fetch(url, { cache: "no-store" });
                if (!res.ok) {
                    showToast(`Skipping "${slug}": ${res.status}`);
                    continue;
                }
                const doc = await res.json();
                state.content = typeof doc.content === "string" ? doc.content : "";
                applyState();
                await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
                const fmt = fmtOverride || currentFormat();
                const canvas = await renderToCanvas(fmt.background);
                const blob = await new Promise((resolve) => canvas.toBlob(resolve, fmt.mime, fmt.quality));
                if (!blob) throw new Error(`Could not encode ${fmt.label}`);
                const blobUrl = URL.createObjectURL(blob);
                const a = document.createElement("a");
                a.href = blobUrl;
                a.download = `${slug}${suffix || ""}.${fmt.ext}`;
                document.body.appendChild(a);
                a.click();
                a.remove();
                setTimeout(() => URL.revokeObjectURL(blobUrl), 1500);
                showToast(`Saved ${a.download}`);
                await new Promise((r) => setTimeout(r, 800));
            } catch (e) {
                showToast(`Export of "${slug}" failed: ${e.message}`);
            }
        }
    } finally {
        state.content = savedContent;
        loadedSlug = savedSlugRef;
        loadedScope = savedScopeRef;
        loadedName = savedNameRef;
        applyState();
        downloadBtn.disabled = false;
    }
}

// Resizable editor pane
const STORAGE_KEY = "terminal-mockup.editorHeight";
const MIN_EDITOR = 80;
const MIN_PREVIEW = 160;
const appRoot = document.querySelector(".app");
const resizeHandle = document.getElementById("resize-handle");

function clampHeight(h) {
    const available = window.innerHeight - document.querySelector(".toolbar").offsetHeight - 6;
    const max = Math.max(MIN_EDITOR, available - MIN_PREVIEW);
    return Math.max(MIN_EDITOR, Math.min(max, h));
}
function setEditorHeight(h) {
    const clamped = clampHeight(h);
    appRoot.style.setProperty("--editor-height", `${clamped}px`);
    return clamped;
}
const saved = Number(localStorage.getItem(STORAGE_KEY));
if (Number.isFinite(saved) && saved > 0) setEditorHeight(saved);

let dragStartY = 0;
let dragStartHeight = 0;
function onPointerMove(e) {
    const dy = e.clientY - dragStartY;
    setEditorHeight(dragStartHeight - dy);
}
function onPointerUp(e) {
    resizeHandle.classList.remove("dragging");
    resizeHandle.releasePointerCapture?.(e.pointerId);
    window.removeEventListener("pointermove", onPointerMove);
    window.removeEventListener("pointerup", onPointerUp);
    const cur = parseInt(getComputedStyle(appRoot).getPropertyValue("--editor-height"), 10);
    if (Number.isFinite(cur)) localStorage.setItem(STORAGE_KEY, String(cur));
}
resizeHandle.addEventListener("pointerdown", (e) => {
    e.preventDefault();
    dragStartY = e.clientY;
    const cs = getComputedStyle(appRoot).getPropertyValue("--editor-height");
    dragStartHeight = parseInt(cs, 10) || 240;
    resizeHandle.classList.add("dragging");
    resizeHandle.setPointerCapture?.(e.pointerId);
    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", onPointerUp);
});
resizeHandle.addEventListener("dblclick", () => {
    setEditorHeight(240);
    localStorage.setItem(STORAGE_KEY, "240");
});
resizeHandle.addEventListener("keydown", (e) => {
    const cs = getComputedStyle(appRoot).getPropertyValue("--editor-height");
    const cur = parseInt(cs, 10) || 240;
    const step = e.shiftKey ? 40 : 12;
    if (e.key === "ArrowUp") { setEditorHeight(cur + step); e.preventDefault(); }
    else if (e.key === "ArrowDown") { setEditorHeight(cur - step); e.preventDefault(); }
    else return;
    const next = parseInt(getComputedStyle(appRoot).getPropertyValue("--editor-height"), 10);
    if (Number.isFinite(next)) localStorage.setItem(STORAGE_KEY, String(next));
});
window.addEventListener("resize", () => {
    const cs = getComputedStyle(appRoot).getPropertyValue("--editor-height");
    const cur = parseInt(cs, 10) || 240;
    setEditorHeight(cur);
});

// Pane visibility toggles
const TOOLBAR_KEY = "terminal-mockup.toolbarCollapsed";
const EDITOR_COLLAPSED_KEY = "terminal-mockup.editorCollapsed";
const toggleToolbarBtn = document.getElementById("toggle-toolbar");
const toggleEditorBtn = document.getElementById("toggle-editor");

function applyToolbarCollapsed(collapsed) {
    appRoot.classList.toggle("toolbar-collapsed", collapsed);
    toggleToolbarBtn.setAttribute("aria-pressed", String(collapsed));
    toggleToolbarBtn.title = collapsed ? "Show toolbar" : "Hide toolbar";
    toggleToolbarBtn.querySelector(".pane-toggle-icon").textContent = collapsed ? "▼" : "▲";
}
function applyEditorCollapsed(collapsed) {
    appRoot.classList.toggle("editor-collapsed", collapsed);
    toggleEditorBtn.setAttribute("aria-pressed", String(collapsed));
    toggleEditorBtn.title = collapsed ? "Show content editor" : "Hide content editor";
    toggleEditorBtn.querySelector(".pane-toggle-icon").textContent = collapsed ? "▲" : "▼";
}
applyToolbarCollapsed(localStorage.getItem(TOOLBAR_KEY) === "1");
applyEditorCollapsed(localStorage.getItem(EDITOR_COLLAPSED_KEY) === "1");
toggleToolbarBtn.addEventListener("click", () => {
    const next = !appRoot.classList.contains("toolbar-collapsed");
    applyToolbarCollapsed(next);
    localStorage.setItem(TOOLBAR_KEY, next ? "1" : "0");
});
toggleEditorBtn.addEventListener("click", () => {
    const next = !appRoot.classList.contains("editor-collapsed");
    applyEditorCollapsed(next);
    localStorage.setItem(EDITOR_COLLAPSED_KEY, next ? "1" : "0");
});

init();
