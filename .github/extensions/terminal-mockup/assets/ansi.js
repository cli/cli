// ANSI SGR + bracket markup tokenizer.
// Produces a flat array of styled segments: { text, classes }.
//
// Supports:
//   - ANSI CSI SGR sequences: \x1b[<params>m  (0, 1, 3, 4, 22, 23, 24, 30-37, 39, 90-97, 38;5;N, 38;2;R;G;B)
//   - Bracket markup: [b]..[/b], [i]..[/i], [u]..[/u], [dim]..[/dim], [muted]..[/muted], [link]..[/link],
//     [red] [green] [yellow] [blue] [magenta] [cyan] [white] [black]
//     [brred] [brgreen] [bryellow] [brblue] [brmagenta] [brcyan] [brwhite] [brblack]
//   - Plain text passthrough
//
// Bracket tags can nest. ANSI state machine handles standard SGR codes only;
// other CSI/OSC sequences are dropped silently.

const ANSI_FG = {
    30: "black", 31: "red", 32: "green", 33: "yellow",
    34: "blue", 35: "magenta", 36: "cyan", 37: "white",
    90: "br-black", 91: "br-red", 92: "br-green", 93: "br-yellow",
    94: "br-blue", 95: "br-magenta", 96: "br-cyan", 97: "br-white",
};

const COLOR_NAMES = new Set([
    "red", "green", "yellow", "blue", "magenta", "cyan", "white", "black",
    "brred", "brgreen", "bryellow", "brblue", "brmagenta", "brcyan", "brwhite", "brblack",
]);
const TAG_TO_FG = {
    red: "red", green: "green", yellow: "yellow", blue: "blue",
    magenta: "magenta", cyan: "cyan", white: "white", black: "black",
    brred: "br-red", brgreen: "br-green", bryellow: "br-yellow", brblue: "br-blue",
    brmagenta: "br-magenta", brcyan: "br-cyan", brwhite: "br-white", brblack: "br-black",
};

function classesFromState(state) {
    const cls = [];
    if (state.fg) cls.push(`fg-${state.fg}`);
    if (state.bold) cls.push("bold");
    if (state.italic) cls.push("italic");
    if (state.underline) cls.push("underline");
    if (state.dim) cls.push("dim");
    return cls;
}

function emit(out, text, state) {
    if (!text) return;
    out.push({ text, classes: classesFromState(state) });
}

// Step 1: parse ANSI escape codes into a flat segment list, ignoring brackets.
function parseAnsi(input) {
    const segments = [];
    const state = { fg: null, bold: false, italic: false, underline: false, dim: false };
    let buf = "";
    let i = 0;
    while (i < input.length) {
        const ch = input.charCodeAt(i);
        if (ch === 0x1b && input[i + 1] === "[") {
            if (buf) { emit(segments, buf, state); buf = ""; }
            // Find terminator
            let j = i + 2;
            while (j < input.length) {
                const c = input.charCodeAt(j);
                // CSI parameter bytes: 0x30-0x3f; intermediates: 0x20-0x2f; final: 0x40-0x7e
                if (c >= 0x40 && c <= 0x7e) break;
                j++;
            }
            const final = input[j];
            const params = input.slice(i + 2, j);
            if (final === "m") applySgr(state, params);
            i = j + 1;
            continue;
        }
        buf += input[i];
        i++;
    }
    if (buf) emit(segments, buf, state);
    return segments;
}

function applySgr(state, paramsStr) {
    const tokens = paramsStr.split(";").map((t) => (t === "" ? 0 : Number(t)));
    let i = 0;
    while (i < tokens.length) {
        const t = tokens[i];
        if (t === 0) {
            state.fg = null; state.bold = false; state.italic = false;
            state.underline = false; state.dim = false;
        } else if (t === 1) state.bold = true;
        else if (t === 2) state.dim = true;
        else if (t === 3) state.italic = true;
        else if (t === 4) state.underline = true;
        else if (t === 22) { state.bold = false; state.dim = false; }
        else if (t === 23) state.italic = false;
        else if (t === 24) state.underline = false;
        else if (t === 39) state.fg = null;
        else if (ANSI_FG[t]) state.fg = ANSI_FG[t];
        else if (t === 38) {
            const mode = tokens[i + 1];
            if (mode === 5) {
                state.fg = map256(tokens[i + 2]);
                i += 2;
            } else if (mode === 2) {
                // Truecolor not mapped to a named slot; skip params and leave fg unchanged.
                i += 4;
            }
        }
        // ignore 40-49, 48 etc (we don't render backgrounds for now)
        i++;
    }
}

// Map 256-color cube to nearest named slot. Coarse but adequate.
function map256(n) {
    if (n == null) return null;
    if (n < 8) return ANSI_FG[30 + n] || null;
    if (n < 16) return ANSI_FG[90 + (n - 8)] || null;
    // Grayscale ramp (232 = near-black, 255 = near-white). The middle range
    // is the "muted" gray that gh uses for footer URLs, bullet separators, etc.
    if (n >= 232 && n <= 243) return "muted";
    if (n >= 244 && n <= 250) return "br-black"; // softer gray
    // Color cube fallback: no good mapping, let the default fg apply.
    return null;
}
// Step 2: walk segments and split on bracket markup, updating per-segment classes.
function parseBrackets(segments) {
    const out = [];
    const stack = []; // each entry: array of class strings added by this tag
    const tagRe = /\[(\/?)([a-zA-Z]+)\]/g;
    for (const seg of segments) {
        const text = seg.text;
        let last = 0;
        tagRe.lastIndex = 0;
        let m;
        const baseClasses = seg.classes.slice();
        while ((m = tagRe.exec(text)) !== null) {
            const before = text.slice(last, m.index);
            if (before) out.push({ text: before, classes: mergeClasses(baseClasses, stack) });
            const closing = m[1] === "/";
            const tag = m[2].toLowerCase();
            const added = tagToClasses(tag);
            if (added.length === 0) {
                // Not a recognized tag; treat as literal text.
                out.push({ text: m[0], classes: mergeClasses(baseClasses, stack) });
            } else if (closing) {
                // Pop most recent matching frame.
                for (let i = stack.length - 1; i >= 0; i--) {
                    if (stack[i].tag === tag) { stack.splice(i, 1); break; }
                }
            } else {
                stack.push({ tag, classes: added });
            }
            last = m.index + m[0].length;
        }
        const tail = text.slice(last);
        if (tail) out.push({ text: tail, classes: mergeClasses(baseClasses, stack) });
    }
    return out;
}

function tagToClasses(tag) {
    if (tag === "b" || tag === "bold") return ["bold"];
    if (tag === "i" || tag === "italic") return ["italic"];
    if (tag === "u" || tag === "underline") return ["underline"];
    if (tag === "dim") return ["dim"];
    if (tag === "muted") return ["fg-muted"];
    if (tag === "link") return ["fg-br-blue", "underline"];
    if (COLOR_NAMES.has(tag)) return [`fg-${TAG_TO_FG[tag]}`];
    return [];
}

function mergeClasses(base, stack) {
    const set = new Set(base);
    for (const frame of stack) {
        for (const c of frame.classes) set.add(c);
    }
    return Array.from(set);
}

// Step 3: optional auto-styling for plain-looking segments.
// Operates only on segments that have no styling yet, to avoid clobbering
// user-specified colors. Splits on detected patterns and inserts styled spans.
function autoStyle(segments) {
    const out = [];
    for (const seg of segments) {
        if (seg.classes.length > 0) {
            out.push(seg);
            continue;
        }
        autoStyleSegment(seg.text, out);
    }
    return out;
}

function autoStyleSegment(text, out) {
    // Process line by line so we can detect $ prompts.
    const lines = text.split(/(\n)/);
    for (const line of lines) {
        if (line === "\n") {
            out.push({ text: "\n", classes: [] });
            continue;
        }
        if (line === "") continue;
        // Prompt line: leading `$ `
        const promptMatch = line.match(/^(\s*)(\$)( )(.*)$/);
        if (promptMatch) {
            const [, leading, dollar, space, rest] = promptMatch;
            if (leading) out.push({ text: leading, classes: [] });
            out.push({ text: dollar, classes: ["fg-muted"] });
            out.push({ text: space, classes: [] });
            // Apply inline auto-stylers to the rest of the prompt line
            autoStyleInline(rest, out);
            continue;
        }
        autoStyleInline(line, out);
    }
}

function autoStyleInline(text, out) {
    // Detect URLs and color/dim them; detect standalone +N/-N tokens for diff stats; detect #NNN refs.
    // Single regex with alternation; iterate over matches.
    const re = /(https?:\/\/[^\s)>\]]+)|(?<![\w/-])([+-]\d+)(?![\w-])|(?<![\w/-])(#\d+)(?![\w-])/g;
    let last = 0;
    let m;
    while ((m = re.exec(text)) !== null) {
        if (m.index > last) out.push({ text: text.slice(last, m.index), classes: [] });
        if (m[1]) {
            out.push({ text: m[1], classes: ["fg-muted"] });
        } else if (m[2]) {
            const cls = m[2].startsWith("+") ? "fg-br-green" : "fg-br-red";
            out.push({ text: m[2], classes: [cls] });
        } else if (m[3]) {
            out.push({ text: m[3], classes: ["fg-br-blue"] });
        }
        last = m.index + m[0].length;
    }
    if (last < text.length) out.push({ text: text.slice(last), classes: [] });
}

export function parse(input, { autoStyle: enableAuto = true } = {}) {
    const ansiSegments = parseAnsi(input ?? "");
    const bracketSegments = parseBrackets(ansiSegments);
    return enableAuto ? autoStyle(bracketSegments) : bracketSegments;
}

export function renderToDom(target, input, opts) {
    const segments = parse(input, opts);
    target.replaceChildren();
    const frag = document.createDocumentFragment();
    for (const seg of segments) {
        if (seg.classes.length === 0) {
            frag.appendChild(document.createTextNode(seg.text));
        } else {
            const span = document.createElement("span");
            span.className = seg.classes.join(" ");
            span.textContent = seg.text;
            frag.appendChild(span);
        }
    }
    target.appendChild(frag);
}
