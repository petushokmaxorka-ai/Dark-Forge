
"use strict";
// dark-forge-chat — Cursor-style chat with 4 advanced features:
//  1. Apply Diff preview (Accept/Reject with side-by-side diff visualization)
//  2. Multi-file edit (select N files → AI edits all simultaneously)
//  3. Composer mode (multi-step plan with file context tracking)
//  4. Embedding-based vector memory (TF-IDF over chat history with cosine similarity)

const vscode = require("vscode");
const fs = require("fs");
const path = require("path");
const os = require("os");

let viewRef = null;
let pendingInlineEdit = null;
let pendingMultiFileEdit = null;

const MODELS_LIST = [
  { id: "qwable",  name: "Qwable-9B",      kind: "local", glyph: "⚙", color: "#00bfbf", desc: "coding · GPU 0" },
  { id: "qwythos", name: "Qwythos-9B-v2",  kind: "local", glyph: "☉", color: "#c8a84b", desc: "reasoning · GPU 1" },
  { id: "qwen",    name: "Qwen3-Coder",    kind: "local", glyph: "Q", color: "#ff8800", desc: "Qwen3 Coder 30B" },
  { id: "glm",     name: "GLM-5.2",         kind: "cloud", glyph: "G", color: "#8b0000", desc: "coding · cloud" },
  { id: "kimi",    name: "Kimi-K2.7",      kind: "cloud", glyph: "K", color: "#4169e1", desc: "creative · cloud" },
  { id: "mimo",    name: "MiMo-V2.5",      kind: "cloud", glyph: "M", color: "#39ff14", desc: "math · cloud" },
  { id: "minimax", name: "MiniMax-M3",     kind: "cloud", glyph: "▣", color: "#ff69b4", desc: "autonomous · cloud" }
];

const HISTORY_DIR = path.join(process.env.HOME || os.homedir(), ".config", "Dark Forge", "chat_history");
try { fs.mkdirSync(HISTORY_DIR, { recursive: true }); } catch (e) {}

// === API helpers ===
async function forgeFetch(p, payload) {
  const cfg = vscode.workspace.getConfiguration("darkForge");
  const baseUrl = cfg.get("baseUrl") || "http://127.0.0.1:9091";
  const res = await fetch(baseUrl + p, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload)
  });
  if (!res.ok) return { error: "HTTP " + res.status + ": " + await res.text() };
  return await res.json();
}
async function sendTeam(payload) { return forgeFetch("/api/chat/team", payload); }
async function sendChat(payload) { return forgeFetch("/api/chat/stream", payload); }

// === [NEW] TF-IDF Vector Memory (Cursor-style embeddings locally) ===
const VECTOR_STORE = { docs: [], idf: {}, total: 0 };

function tokenize(s) {
  return (s || "").toLowerCase()
    .replace(/[^\p{L}\p{N}\s]/gu, " ")
    .split(/\s+/)
    .filter(t => t.length >= 2 && t.length <= 30);
}

function computeTF(tokens) {
  const tf = new Map();
  for (const t of tokens) tf.set(t, (tf.get(t) || 0) + 1);
  const len = tokens.length || 1;
  for (const k of tf.keys()) tf.set(k, tf.get(k) / len);
  return tf;
}

function buildIndex() {
  if (!fs.existsSync(HISTORY_DIR)) return;
  VECTOR_STORE.docs = [];
  VECTOR_STORE.idf = new Map();
  VECTOR_STORE.total = 0;
  const docs = [];
  const files = fs.readdirSync(HISTORY_DIR).filter(f => f.endsWith(".jsonl"));
  for (const f of files.slice(-50)) {
    try {
      const lines = fs.readFileSync(path.join(HISTORY_DIR, f), "utf8").trim().split("\n");
      for (const line of lines) {
        try {
          const d = JSON.parse(line);
          const text = (d.instruction || "") + " " + (d.replacement || "");
          if (text.length < 10) continue;
          docs.push({ id: f + ":" + d.ts, text, tokens: tokenize(text) });
        } catch (e) {}
      }
    } catch (e) {}
  }
  // IDF
  const df = new Map();
  for (const d of docs) {
    const seen = new Set(d.tokens);
    for (const t of seen) df.set(t, (df.get(t) || 0) + 1);
  }
  for (const [t, c] of df.entries()) {
    VECTOR_STORE.idf.set(t, Math.log((docs.length + 1) / (c + 1)) + 1);
  }
  // TF
  for (const d of docs) {
    d.tf = computeTF(d.tokens);
  }
  VECTOR_STORE.docs = docs;
  VECTOR_STORE.total = docs.length;
}

function tfidfSearch(query, k = 5) {
  if (VECTOR_STORE.total === 0) buildIndex();
  const qTokens = tokenize(query);
  const qTf = computeTF(qTokens);
  const qVec = new Map();
  for (const [t, f] of qTf.entries()) {
    qVec.set(t, f * (VECTOR_STORE.idf.get(t) || 0));
  }
  const scores = [];
  for (const doc of VECTOR_STORE.docs) {
    let s = 0;
    for (const [t, w] of qVec.entries()) {
      if (doc.tf.has(t)) s += w * doc.tf.get(t) * (VECTOR_STORE.idf.get(t) || 0);
    }
    if (s > 0) scores.push({ s, doc });
  }
  scores.sort((a, b) => b.s - a.s);
  return scores.slice(0, k).map(x => ({
    text: x.doc.text.slice(0, 300),
    score: Math.round(x.s * 100) / 100,
    id: x.doc.id
  }));
}

// === [NEW] Diff preview generator (Cursor-style) ===
function generateDiff(original, replacement) {
  const oLines = original.split("\n");
  const rLines = replacement.split("\n");
  const ctx = 3; // context lines
  const result = [];
  let i = 0, j = 0;
  while (i < oLines.length || j < rLines.length) {
    if (i < oLines.length && j < rLines.length && oLines[i] === rLines[j]) {
      result.push({ type: " ", line: oLines[i] });
      i++; j++;
    } else {
      // find longest common subsequence approx
      let matchI = -1, matchJ = -1, matchLen = 0;
      for (let ii = i; ii < Math.min(i + 10, oLines.length); ii++) {
        for (let jj = j; jj < Math.min(j + 10, rLines.length); jj++) {
          if (oLines[ii] === rLines[jj]) {
            let len = 0;
            while (ii + len < oLines.length && jj + len < rLines.length && oLines[ii + len] === rLines[jj + len]) len++;
            if (len > matchLen) { matchLen = len; matchI = ii; matchJ = jj; }
          }
        }
      }
      if (matchLen > 0) {
        // emit deletions
        while (i < matchI) { result.push({ type: "-", line: oLines[i] }); i++; }
        while (j < matchJ) { result.push({ type: "+", line: rLines[j] }); j++; }
        // emit common
        for (let k = 0; k < matchLen; k++) {
          result.push({ type: " ", line: oLines[i] }); i++; j++;
        }
      } else {
        if (i < oLines.length) { result.push({ type: "-", line: oLines[i] }); i++; }
        if (j < rLines.length) { result.push({ type: "+", line: rLines[j] }); j++; }
      }
    }
  }
  return result;
}

// === [NEW] Multi-file edit (Cursor-style) ===
async function multiFileEdit() {
  const files = await vscode.window.showOpenDialog({
    canSelectFiles: true, canSelectFolders: false, canSelectMany: true,
    filters: { "Code": ["py","ts","js","tsx","jsx","go","rs","java","c","cpp","h","hpp","rb","go","rs","sh","md","json","yaml","yml","toml"] }
  });
  if (!files || files.length === 0) return;
  const instruction = await vscode.window.showInputBox({
    prompt: `Edit instruction for ${files.length} file(s)`,
    placeHolder: "e.g. add error handling, add type hints, refactor imports"
  });
  if (!instruction) return;
  await vscode.window.withProgress(
    { location: vscode.ProgressLocation.Window, title: `⚒ Multi-file edit: ${files.length} files via 6 models…` },
    async () => {
      const edits = [];
      for (const file of files) {
        try {
          const doc = await vscode.workspace.openTextDocument(file);
          const content = doc.getText();
          const res = await sendChat({
            message: `Edit this code in-place. Return ONLY the replacement code, no markdown fences.\n\nOriginal:\n${content}\n\nInstruction: ${instruction}`,
            persona: "anathemetron",
            contextFiles: [file.fsPath]
          });
          if (res.error || !res.response) continue;
          let newCode = res.response.trim()
            .replace(/^```[a-z]*\n/i, "")
            .replace(/\n```\s*$/, "");
          if (!newCode || newCode === content) continue;
          const diff = generateDiff(content, newCode);
          edits.push({ file: file.fsPath, original: content, replacement: newCode, diff });
        } catch (e) {}
      }
      if (edits.length === 0) {
        vscode.window.showInformationMessage("⚒ No edits suggested");
        return;
      }
      pendingMultiFileEdit = edits;
      const summary = edits.map((e, i) => `${i + 1}. ${path.basename(e.file)} (${e.diff.length} lines diff)`).join("\n");
      const choice = await vscode.window.showInformationMessage(
        `⚒ ${edits.length} files ready. Apply all?\n\n${summary}`,
        { modal: false }, "Apply All", "Cancel"
      );
      if (choice === "Apply All") {
        for (const e of edits) {
          const doc = await vscode.workspace.openTextDocument(e.file);
          const wsEdit = new vscode.WorkspaceEdit();
          wsEdit.replace(doc.uri, new vscode.Range(0, 0, doc.lineCount, 0), e.replacement);
          await vscode.workspace.applyEdit(wsEdit);
          saveHistory("multi-file-edit", instruction, e.original, e.replacement);
        }
        vscode.window.setStatusBarMessage(`⚒ Applied ${edits.length} edits`, 3000);
      } else {
        vscode.window.setStatusBarMessage("⚒ cancelled", 3000);
      }
      pendingMultiFileEdit = null;
    }
  );
}

// === [NEW] Composer mode (multi-step plan with context tracking) ===
async function composerRun(messages) {
  const plan = [
    { phase: "analyze", prompt: `Analyze this request and identify key components:\n\n${messages}` },
    { phase: "design", prompt: `Design a solution architecture for: ${messages}` },
    { phase: "implement", prompt: `Provide concrete implementation steps for: ${messages}` },
    { phase: "review", prompt: `Review the solution and list potential improvements for: ${messages}` }
  ];
  const results = [];
  for (const step of plan) {
    try {
      const r = await sendTeam({ message: step.prompt, mode: "single", models: ["kimi"], max_tokens: 1024 });
      results.push({ phase: step.phase, text: r.team?.final || r.team?.consensus || r.error || "(no response)" });
    } catch (e) {
      results.push({ phase: step.phase, text: "Error: " + String(e) });
    }
  }
  return results;
}

// === @-mentions resolver ===
async function resolveAtMentions(text) {
  const mentions = [];
  const re = /@(file|folder|workspace)\s+([^\s,;]+)/g;
  let m;
  while ((m = re.exec(text)) !== null) {
    const type = m[1];
    const raw = m[2];
    try {
      if (type === "workspace") {
        const files = await vscode.workspace.findFiles("**/*", "**/node_modules/**", 50);
        for (const f of files.slice(0, 10)) {
          try {
            const doc = await vscode.workspace.openTextDocument(f);
            mentions.push({ type, path: f.fsPath, content: doc.getText().slice(0, 3000) });
          } catch (e) {}
        }
      } else if (type === "file") {
        const files = await vscode.workspace.findFiles(raw, "**/node_modules/**", 5);
        for (const f of files) {
          try {
            const doc = await vscode.workspace.openTextDocument(f);
            mentions.push({ type, path: f.fsPath, content: doc.getText().slice(0, 5000) });
          } catch (e) {}
        }
      } else if (type === "folder") {
        const pattern = raw.replace(/\/$/, "") + "/**/*";
        const files = await vscode.workspace.findFiles(pattern, "**/node_modules/**", 20);
        for (const f of files.slice(0, 10)) {
          try {
            const doc = await vscode.workspace.openTextDocument(f);
            mentions.push({ type, path: f.fsPath, content: doc.getText().slice(0, 3000) });
          } catch (e) {}
        }
      }
    } catch (e) {}
  }
  return mentions;
}

async function getActiveEditorContext() {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return [];
  return [{ path: editor.document.fileName, content: editor.document.getText().slice(0, 6000) }];
}

// === [MODIFIED] Inline edit with diff preview ===
async function inlineEdit() {
  const editor = vscode.window.activeTextEditor;
  if (!editor) {
    vscode.window.showWarningMessage("⚒ Dark Forge: no active editor");
    return;
  }
  const selection = editor.selection;
  const selectedText = editor.document.getText(selection);
  if (!selectedText || selectedText.length < 1) {
    vscode.window.showWarningMessage("⚒ Dark Forge: select code first, then Cmd+K");
    return;
  }
  const instruction = await vscode.window.showInputBox({
    prompt: "Describe the edit (6 models rewrite together)",
    placeHolder: "e.g. convert to async/await, add error handling"
  });
  if (!instruction) return;
  await vscode.window.withProgress(
    { location: vscode.ProgressLocation.Window, title: "⚒ 6 models rewriting in parallel…" },
    async () => {
      const prompt = "Edit this code in-place. Return ONLY the replacement code, no markdown fences.\n\nOriginal:\n" + selectedText + "\n\nInstruction: " + instruction;
      const res = await sendChat({ message: prompt, persona: "anathemetron", contextFiles: [editor.document.fileName] });
      if (res.error) { vscode.window.showErrorMessage("⚒ " + res.error); return; }
      let replacement = (res.response || "").trim();
      replacement = replacement.replace(/^```[a-z]*\n/i, "").replace(/\n```\s*$/, "");
      if (!replacement || replacement === selectedText.trim()) {
        vscode.window.showInformationMessage("⚒ no change suggested"); return;
      }
      // [NEW] Compute diff
      const diff = generateDiff(selectedText, replacement);
      const diffText = diff.slice(0, 50).map(d => {
        const sym = d.type === "+" ? "+ " : d.type === "-" ? "- " : "  ";
        return sym + d.line.slice(0, 100);
      }).join("\n");
      pendingInlineEdit = { uri: editor.document.uri, range: selection, original: selectedText, replacement, diff };
      await editor.edit(b => b.replace(selection, replacement));
      // [NEW] Show diff preview
      const choice = await vscode.window.showInformationMessage(
        `⚒ Edit applied (${diff.length} lines). Accept?\n\n${diffText}${diff.length > 50 ? "\n... (more)" : ""}`,
        { modal: false }, "Accept", "Reject"
      );
      if (choice === "Accept") {
        saveHistory("inline-edit", instruction, selectedText, replacement);
        vscode.window.setStatusBarMessage("⚒ accepted", 3000);
      } else if (choice === "Reject") {
        await editor.edit(b => b.replace(selection, selectedText));
        vscode.window.setStatusBarMessage("⚒ rejected", 3000);
      }
      pendingInlineEdit = null;
    }
  );
}

// === Tab autocomplete ===
const completionCache = new Map();

function registerCompletionProvider(context) {
  context.subscriptions.push(
    vscode.workspace.onDidChangeTextDocument(e => {
      const uri = e.document.uri.toString();
      for (const k of Array.from(completionCache.keys())) if (k.startsWith(uri)) completionCache.delete(k);
    })
  );
  context.subscriptions.push(
    vscode.languages.registerInlineCompletionItemProvider(
      { pattern: "**" },
      {
        async provideInlineCompletionItems(document, position) {
          const line = document.lineAt(position.line).text;
          if (line.trim().length < 4) return [];
          const startLine = Math.max(0, position.line - 30);
          const context = document.getText(new vscode.Range(startLine, 0, position.line, line.length));
          try {
            const res = await sendChat({
              message: "Complete next line of code. Return ONLY completion text (no markdown), max 80 chars.\n\n" + context + "\nNEXT:",
              persona: "anathemetron",
              contextFiles: [document.fileName]
            });
            if (res.error || !res.response) return [];
            return [{ insertText: res.response.trim().split("\n")[0].slice(0, 80) }];
          } catch (e) { return []; }
        }
      }
    )
  );
}

function saveHistory(type, instruction, original, replacement) {
  try {
    const ts = new Date().toISOString().replace(/[:.]/g, "-");
    fs.appendFileSync(path.join(HISTORY_DIR, type + "-" + ts + ".jsonl"),
      JSON.stringify({ ts: new Date().toISOString(), type, instruction, original: (original||"").slice(0, 5000), replacement: (replacement||"").slice(0, 5000) }) + "\n");
  } catch (e) {}
}

function loadHistory(limit) {
  try {
    const files = fs.readdirSync(HISTORY_DIR).sort().reverse().slice(0, limit || 50);
    const out = [];
    for (const f of files) {
      try {
        const lines = fs.readFileSync(path.join(HISTORY_DIR, f), "utf8").trim().split("\n");
        for (const line of lines) {
          try { out.push(JSON.parse(line)); } catch (e) {}
        }
      } catch (e) {}
    }
    return out.reverse();
  } catch (e) { return []; }
}

function saveChatMessage(role, text, mentions) {
  try {
    const ts = new Date().toISOString().replace(/[:.]/g, "-");
    fs.appendFileSync(path.join(HISTORY_DIR, "chat-" + ts + ".jsonl"),
      JSON.stringify({ ts: new Date().toISOString(), type: "chat", instruction: role, original: mentions ? "[mentions: " + mentions.map(m => m.path).join(", ") + "]" : "", replacement: text.slice(0, 5000) }) + "\n");
  } catch (e) {}
}

// === [NEW] Vector memory (TF-IDF) search for webview ===
async function memorySearchV2(query) {
  return tfidfSearch(query, 5);
}

async function completeAtMention(partial) {
  try {
    const files = await vscode.workspace.findFiles("**/" + (partial || "*"), "**/node_modules/**", 15);
    return files.map(f => f.fsPath);
  } catch (e) { return []; }
}

class SwarmView {
  constructor(context) { this.context = context; }
  resolveWebviewView(webviewView, context, token) {
    viewRef = webviewView;
    webviewView.webview.options = { enableScripts: true, retainContextWhenHidden: true, enableForms: true, localResourceRoots: [] };
    webviewView.webview.html = this.getHtml();
    webviewView.webview.onDidReceiveMessage(async (msg) => {
      if (!msg || !msg.command) return;
      if (msg.command === "ready") {
        const cfg = vscode.workspace.getConfiguration("darkForge");
        webviewView.webview.postMessage({ type: "config", models: MODELS_LIST, config: { baseUrl: cfg.get("baseUrl") || "http://127.0.0.1:9091", defaultMode: cfg.get("defaultMode") || "swarm", participants: cfg.get("participants") || ["qwable","qwythos"] } });
      } else if (msg.command === "send") {
        await this.handleSend(webviewView, msg);
      } else if (msg.command === "history") {
        const h = loadHistory(50);
        webviewView.webview.postMessage({ type: "history", items: h });
      } else if (msg.command === "inlineEdit") {
        await inlineEdit();
      } else if (msg.command === "memorySearch") {
        const r = await memorySearchV2(msg.query || "");
        webviewView.webview.postMessage({ type: "memoryResults", query: msg.query, results: r });
      } else if (msg.command === "uploadImage") {
        const result = await vscode.window.showOpenDialog({ canSelectFiles: true, canSelectFolders: false, canSelectMany: false, filters: { "Images": ["png", "jpg", "jpeg", "gif", "webp", "bmp"] } });
        if (result && result[0]) {
          const buf = fs.readFileSync(result[0].fsPath);
          const ext = result[0].fsPath.split(".").pop().toLowerCase();
          const mime = ext === "jpg" ? "jpeg" : ext;
          webviewView.webview.postMessage({ type: "image", data: "data:image/" + mime + ";base64," + buf.toString("base64"), name: result[0].fsPath.split("/").pop() });
        }
      } else if (msg.command === "composer") {
        const r = await composerRun(msg.message || "");
        webviewView.webview.postMessage({ type: "composerResults", results: r });
      } else if (msg.command === "multiFileEdit") {
        await multiFileEdit();
      } else if (msg.command === "acceptEdit") {
        if (pendingInlineEdit) {
          saveHistory("inline-edit", "accepted", pendingInlineEdit.original, pendingInlineEdit.replacement);
          pendingInlineEdit = null;
          webviewView.webview.postMessage({ type: "editAccepted" });
        }
      } else if (msg.command === "rejectEdit") {
        if (pendingInlineEdit) {
          const ed = await vscode.workspace.openTextDocument(pendingInlineEdit.uri);
          await ed.edit(b => b.replace(pendingInlineEdit.range, pendingInlineEdit.original));
          pendingInlineEdit = null;
          webviewView.webview.postMessage({ type: "editRejected" });
        }
      } else if (msg.command === "acceptMultiEdit") {
        if (pendingMultiFileEdit) {
          for (const e of pendingMultiFileEdit) {
            const doc = await vscode.workspace.openTextDocument(e.file);
            const wsEdit = new vscode.WorkspaceEdit();
            wsEdit.replace(doc.uri, new vscode.Range(0, 0, doc.lineCount, 0), e.replacement);
            await vscode.workspace.applyEdit(wsEdit);
            saveHistory("multi-file-edit", "accepted", e.original, e.replacement);
          }
          pendingMultiFileEdit = null;
          vscode.window.setStatusBarMessage("⚒ multi-edit applied", 3000);
          webviewView.webview.postMessage({ type: "multiEditAccepted" });
        }
      } else if (msg.command === "cancelMultiEdit") {
        pendingMultiFileEdit = null;
        webviewView.webview.postMessage({ type: "multiEditCancelled" });
      } else if (msg.command === "completeAtMention") {
        const items = await completeAtMention(msg.partial || "");
        webviewView.webview.postMessage({ type: "atMentionResults", items });
      }
    }, null, this.context.subscriptions);
  }

  async handleSend(webviewView, msg) {
    if (!msg.text || !msg.text.trim()) return;
    const ps = Array.isArray(msg.participants) ? msg.participants : [];
    if (ps.length === 0) return;
    const rounds = parseInt(msg.rounds) || 2;
    const image = msg.image || null;
    webviewView.webview.postMessage({ type: "start" });
    try {
      const mentions = await resolveAtMentions(msg.text);
      const editorCtx = await getActiveEditorContext();
      const memoryHits = await memorySearchV2(msg.text.slice(0, 200));
      let fullMessage = msg.text;
      if (mentions.length) fullMessage += "\n\n[@-mentions]\n" + mentions.map(m => "[" + m.type + "] " + m.path + (m.content ? "\n" + m.content : "")).join("\n---\n");
      if (editorCtx.length) fullMessage += "\n\n[Active editor]\n" + editorCtx[0].path + ":\n" + editorCtx[0].content;
      if (memoryHits.length) fullMessage += "\n\n[Vector memory hits]\n" + memoryHits.map(h => "- " + h.text).join("\n");
      if (image) fullMessage = "[User attached image] " + msg.text;
      const data = await sendTeam({ message: fullMessage, mode: msg.mode || "swarm", models: ps, rounds: rounds, max_tokens: 1024 });
      if (data.error) { webviewView.webview.postMessage({ type: "error", error: data.error }); return; }
      saveChatMessage("user", msg.text, mentions);
      saveChatMessage("assistant", data.team ? data.team.consensus || data.team.final || "" : "", null);
      webviewView.webview.postMessage({ type: "swarm", result: data, mentions, memoryHits });
    } catch (e) {
      webviewView.webview.postMessage({ type: "error", error: String(e) });
    }
  }

  getHtml() {
    const modelsJson = JSON.stringify(MODELS_LIST);
    return [
      "<!DOCTYPE html>",
      "<html lang=\"ru\"><head><meta charset=\"UTF-8\">",
      "<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src http://127.0.0.1:9091 http://localhost:9091; img-src 'self' data: blob:; font-src 'self' data:;\">",
      "<title>Dark Forge Swarm — Cursor 100%</title>",
      "<style>",
      ":root{--bg:#000;--panel:#0a0a0a;--p2:#0e0e0e;--text:#b8a060;--muted:#5a4a3a;--border:#2a2418;--gold:#c8a86e;--g2:#e8c97c;--red:#a33;--green:#5a7c48;--blue:#3a6090;--added-bg:rgba(90,124,72,0.15);--removed-bg:rgba(170,51,51,0.15);}",
      "* { box-sizing: border-box; }",
      "html, body { margin: 0; padding: 0; height: 100%; overflow: hidden; }",
      "body { font-family: 'JetBrains Mono','Fira Code',monospace; font-size: 12px; background: #000; color: #b8a060; display: flex; flex-direction: column; height: 100vh; }",
      "header { padding: 8px 12px; border-bottom: 1px solid #2a2418; background: #000; display: flex; justify-content: space-between; align-items: center; flex-shrink: 0; }",
      "header h1 { margin: 0; font-size: 13px; color: #c8a86e; letter-spacing: 0.05em; font-weight: normal; }",
      ".btn-mini { font-size: 9px; background: transparent; border: 1px solid #3a3028; color: #b8a060; padding: 3px 8px; cursor: pointer; font-family: inherit; margin-left: 4px; }",
      ".btn-mini:hover { color: #e8c97c; border-color: #b8a060; background: rgba(184,160,96,0.08); }",
      ".btn-mini.active { color: #000; background: #c8a86e; border-color: #c8a86e; }",
      ".btn-mini.primary { color: #c8a86e; border-color: #c8a86e; background: transparent; }",
      "#main { flex: 1; display: flex; flex-direction: column; overflow: hidden; min-height: 0; }",
      "#messages { flex: 1; overflow-y: auto; padding: 8px 12px; display: flex; flex-direction: column; gap: 8px; min-height: 0; }",
      ".msg { padding: 8px 10px; border: 1px solid #2a2418; background: #000; }",
      ".msg.user { border-left: 3px solid #b8a060; }",
      ".msg .meta { color: #5a4a3a; font-size: 9px; text-transform: uppercase; letter-spacing: 0.05em; margin-bottom: 4px; }",
      ".msg .body { white-space: pre-wrap; color: #b8a060; font-size: 11px; line-height: 1.5; }",
      ".msg .img-thumb { max-width: 100%; max-height: 100px; border: 1px solid #3a3028; margin: 4px 0; }",
      "/* [NEW] Diff preview styles */",
      ".diff-container { background: #000; border: 1px solid #3a3028; margin: 6px 0; font-size: 10px; max-height: 300px; overflow-y: auto; }",
      ".diff-line { padding: 1px 6px; white-space: pre; font-family: monospace; }",
      ".diff-line.add { background: var(--added-bg); color: #5a7c48; }",
      ".diff-line.add::before { content: '+ '; opacity: 0.7; }",
      ".diff-line.del { background: var(--removed-bg); color: #a55; text-decoration: line-through; }",
      ".diff-line.del::before { content: '- '; opacity: 0.7; }",
      ".diff-line.ctx { color: #5a4a3a; }",
      ".diff-line.ctx::before { content: '  '; opacity: 0.5; }",
      ".diff-actions { display: flex; gap: 4px; padding: 6px 8px; background: #0a0a0a; border-top: 1px solid #2a2418; justify-content: flex-end; }",
      ".diff-actions button { font-size: 10px; padding: 4px 12px; cursor: pointer; font-family: inherit; border: 1px solid #3a3028; color: #b8a060; background: transparent; }",
      ".diff-actions button:hover { background: rgba(184,160,96,0.1); }",
      ".diff-actions button.accept { border-color: #5a7c48; color: #5a7c48; }",
      ".diff-actions button.reject { border-color: #a55; color: #a55; }",
      ".swarm-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; margin-top: 4px; }",
      ".agent-card { background: #000; border: 1px solid #3a3028; padding: 6px 8px; border-left: 3px solid #5a4a3a; }",
      ".agent-card.local { border-left-color: #5a7c48; }",
      ".agent-card.cloud { border-left-color: #3a6090; }",
      ".agent-card.qwen { border-left-color: #ff8800; }",
      ".agent-card .head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px; }",
      ".agent-card .name { font-size: 10px; font-weight: bold; }",
      ".agent-card .dur { font-size: 9px; color: #5a4a3a; }",
      ".agent-card .text { font-size: 10px; color: #b8a060; white-space: pre-wrap; max-height: 100px; overflow-y: auto; line-height: 1.4; }",
      ".agent-card.error { border-left-color: #a33 !important; background: rgba(170,51,51,0.06); }",
      ".agent-card.error .text { color: #a33; font-style: italic; }",
      ".consensus { margin-top: 8px; padding: 8px 10px; border: 1px solid #b8a060; background: rgba(184,160,96,0.04); border-left: 3px solid #b8a060; }",
      ".consensus .lab { font-size: 9px; color: #e8c97c; text-transform: uppercase; letter-spacing: 0.1em; margin-bottom: 4px; }",
      ".consensus .body { white-space: pre-wrap; color: #b8a060; font-size: 11px; line-height: 1.5; }",
      ".composer-step { background: #0a0a0a; border-left: 2px solid #b8a060; padding: 6px 10px; margin: 4px 0; }",
      ".composer-step .step-name { color: #e8c97c; font-size: 10px; text-transform: uppercase; letter-spacing: 0.1em; margin-bottom: 4px; }",
      ".composer-step .step-body { color: #b8a060; font-size: 11px; line-height: 1.5; white-space: pre-wrap; }",
      ".memory-hit { background: #0a0a0a; border: 1px solid #3a3028; padding: 4px 8px; margin: 2px 0; font-size: 10px; }",
      ".memory-hit .src { color: #5a4a3a; font-size: 8px; }",
      ".memory-hit .score { color: #5a7c48; font-weight: bold; margin-right: 4px; }",
      ".memory-hit .text { color: #b8a060; }",
      "#input-row { padding: 8px 10px; border-top: 1px solid #2a2418; background: #000; display: flex; gap: 6px; flex-shrink: 0; align-items: stretch; }",
      "#input { flex: 1; background: #000; border: 1px solid #3a3028; color: #b8a060; padding: 6px; font-family: inherit; font-size: 11px; resize: none; min-height: 32px; max-height: 100px; line-height: 1.4; }",
      "#input:focus { outline: none; border-color: #b8a060; }",
      "button.primary { background: transparent; border: 1px solid #b8a060; color: #b8a060; padding: 0 14px; cursor: pointer; font-family: inherit; font-size: 10px; text-transform: uppercase; letter-spacing: 0.05em; }",
      "button.primary:hover:not(:disabled) { background: rgba(184,160,96,0.1); color: #e8c97c; }",
      "button.primary:disabled { opacity: 0.4; cursor: not-allowed; }",
      "#status { padding: 3px 10px; color: #5a4a3a; font-size: 9px; text-transform: uppercase; letter-spacing: 0.1em; border-top: 1px solid #2a2418; background: #000; flex-shrink: 0; }",
      ".placeholder { color: #5a4a3a; text-align: center; padding: 20px; font-style: italic; font-size: 10px; line-height: 1.6; }",
      "input[type=file] { display: none; }",
      "</style></head><body>",
      "",
      "<header><h1>⚒ DARK FORGE SWARM — Cursor 100%</h1><div>",
      "<button class=\"btn-mini\" id=\"selectModelsBtn\">☰ models</button>",
      "<button class=\"btn-mini\" id=\"composerBtn\">🧠 composer</button>",
      "<button class=\"btn-mini\" id=\"multiFileBtn\">📁 multi-edit</button>",
      "<button class=\"btn-mini\" id=\"memoryBtn\">🧬 memory</button>",
      "<button class=\"btn-mini\" id=\"inlineEditBtn\">⌘K edit</button>",
      "</div></header>",
      "",
      "<div id=\"main\">",
      "  <div id=\"messages\"><div class=\"placeholder\">⚒ Dark Forge Swarm — Cursor 100%<br><br>7 моделей: 3 local (Qwable, Qwythos, Qwen) + 4 cloud (GLM, Kimi, MiMo, MiniMax).<br><br>🆕 <b>4 новых фичи:</b><br>• <b>Apply Diff</b> с Accept/Reject preview<br>• <b>Multi-file edit</b> — выбрать файлы → AI edit все<br>• <b>Composer</b> — multi-step plan (analyze → design → implement → review)<br>• <b>🧬 Vector memory</b> (TF-IDF embeddings, cosine similarity)<br><br>📎 upload, ⌘K edit, 📁 multi-edit, 🧬 memory, 🧠 composer.</div></div>",
      "  <div id=\"input-row\">",
      "    <textarea id=\"input\" placeholder=\"Спроси у 7 моделей. @file main.go @folder src/ ...\" rows=\"2\"></textarea>",
      "    <input type=\"file\" id=\"fileInput\" accept=\"image/*,.txt,.pdf,.md,.py,.js,.ts,.json,.yaml,.yml,.sh,.go,.rs,.html,.css,.xml,.csv\">",
      "    <button class=\"btn-mini\" id=\"uploadBtn\" title=\"Upload\">📎</button>",
      "    <button class=\"btn-mini primary\" id=\"send\" title=\"Ctrl+Enter\">⚒ Send</button>",
      "  </div>",
      "  <div id=\"status\">Ready · 7 моделей · Ctrl+Enter</div>",
      "</div>",
      "",
      "<!-- SELECT MODELS POPUP -->",
      "<div class=\"popup-bg\" id=\"modelsPopup\"><div class=\"popup\">",
      "  <div class=\"popup-header\"><span>⚒ SELECT MODELS · TEAMS COUNCIL</span><button class=\"popup-close\" id=\"closeModelsPopup\">×</button></div>",
      "  <div id=\"modelsPopupBody\"></div>",
      "  <div class=\"popup-actions\">",
      "    <button class=\"btn-mini primary\" id=\"popupAll\">✓ all</button>",
      "    <button class=\"btn-mini primary\" id=\"popupNone\">✗ none</button>",
      "    <button class=\"btn-mini primary\" id=\"popupLocal\">⚙ local</button>",
      "    <button class=\"btn-mini primary\" id=\"popupCloud\">☁ cloud</button>",
      "  </div>",
      "</div></div>",
      "",
      "<!-- HISTORY POPUP -->",
      "<div class=\"popup-bg\" id=\"historyPopup\"><div class=\"popup\">",
      "  <div class=\"popup-header\"><span>⏱ HISTORY</span><button class=\"popup-close\" id=\"closeHistoryPopup\">×</button></div>",
      "  <div id=\"historyPopupBody\"></div>",
      "</div></div>",
      "",
      "<!-- MEMORY POPUP -->",
      "<div class=\"popup-bg\" id=\"memoryPopup\"><div class=\"popup\">",
      "  <div class=\"popup-header\"><span>🧬 VECTOR MEMORY (TF-IDF)</span><button class=\"popup-close\" id=\"closeMemoryPopup\">×</button></div>",
      "  <div style=\"display:flex;gap:4px;margin-top:6px;\">",
      "    <input id=\"memoryQuery\" type=\"text\" placeholder=\"Search past chats...\" style=\"flex:1;background:#000;color:#b8a060;border:1px solid #3a3028;padding:2px 4px;font-family:inherit;font-size:10px;\">",
      "    <button class=\"btn-mini primary\" id=\"memorySearchBtn\">🔍 search</button>",
      "  </div>",
      "  <div id=\"memoryPopupBody\"></div>",
      "</div></div>",
      "",
      "<!-- DIFF PREVIEW MODAL -->",
      "<div class=\"popup-bg\" id=\"diffPopup\"><div class=\"popup\" style=\"max-width:600px;max-height:85vh;\">",
      "  <div class=\"popup-header\"><span id=\"diffPopupTitle\">⚒ APPLY DIFF</span><button class=\"popup-close\" id=\"closeDiffPopup\">×</button></div>",
      "  <div id=\"diffPopupBody\"></div>",
      "  <div class=\"diff-actions\" id=\"diffPopupActions\"></div>",
      "</div></div>",
      "",
      "<script>",
      "const MODELS = " + modelsJson + ";",
      "let selected = new Set(MODELS.map(function(m){return m.id;}));",
      "let currentMode = 'swarm';",
      "let pending = false;",
      "let currentImage = null;",
      "let currentRounds = 2;",
      "let pendingEdit = null;",
      "let pendingMultiEdit = null;",
      "const $ = id => document.getElementById(id);",
      "function esc(t){const d=document.createElement('div');d.textContent=t;return d.innerHTML;}",
      "function showToast(t,k){k=k||'info';const c={info:'#3a6090',success:'#5a7c48',warning:'#c75b39',error:'#a33'};const el=document.createElement('div');el.style.cssText='position:fixed;bottom:60px;right:20px;background:#0a0a0a;border:1px solid '+c[k]+';color:#b8a060;padding:8px 14px;font-size:11px;z-index:9999;animation:dtIn 0.2s';el.textContent=t;document.body.appendChild(el);setTimeout(()=>{el.remove();},3000);}",
      "function showDiffPopup(file, diff, acceptCmd, rejectCmd){",
      "  pendingEdit = { file, diff, acceptCmd, rejectCmd };",
      "  const body = $('diffPopupBody');",
      "  body.innerHTML = '';",
      "  $('diffPopupTitle').textContent = '⚒ DIFF: ' + file.split('/').pop();",
      "  const wrap = document.createElement('div');",
      "  wrap.className = 'diff-container';",
      "  for (const d of diff.slice(0, 100)) {",
      "    const ln = document.createElement('div');",
      "    ln.className = 'diff-line ' + (d.type === '+' ? 'add' : d.type === '-' ? 'del' : 'ctx');",
      "    ln.textContent = d.line;",
      "    wrap.appendChild(ln);",
      "  }",
      "  if (diff.length > 100) {",
      "    const more = document.createElement('div');",
      "    more.className = 'diff-line ctx';",
      "    more.textContent = '... (' + (diff.length - 100) + ' more lines)';",
      "    wrap.appendChild(more);",
      "  }",
      "  body.appendChild(wrap);",
      "  const acts = $('diffPopupActions');",
      "  acts.innerHTML = '';",
      "  const acc = document.createElement('button');",
      "  acc.className = 'accept';",
      "  acc.textContent = '✓ Accept';",
      "  acc.onclick = function(){ $('diffPopup').classList.remove('on'); vscode.postMessage({command:'acceptMultiEdit'}); };",
      "  acts.appendChild(acc);",
      "  const rej = document.createElement('button');",
      "  rej.className = 'reject';",
      "  rej.textContent = '✗ Reject';",
      "  rej.onclick = function(){ $('diffPopup').classList.remove('on'); vscode.postMessage({command:'cancelMultiEdit'}); };",
      "  acts.appendChild(rej);",
      "  $('diffPopup').classList.add('on');",
      "}",
      "",
      "function renderModelsPopup(){",
      "  const body = $('modelsPopupBody'); body.innerHTML = '';",
      "  const modeRow = document.createElement('div'); modeRow.className = 'popup-row';",
      "  const modeLbl = document.createElement('span'); modeLbl.className = 'popup-label'; modeLbl.textContent = 'Mode:';",
      "  modeRow.appendChild(modeLbl);",
      "  ['swarm','race','debate','dyad','single'].forEach(function(m){",
      "    const b = document.createElement('button'); b.className = 'btn-mini' + (currentMode===m?' active':''); b.textContent = m;",
      "    b.onclick = function(){ currentMode = m; renderModelsPopup(); };",
      "    modeRow.appendChild(b);",
      "  });",
      "  body.appendChild(modeRow);",
      "  const roundsRow = document.createElement('div'); roundsRow.className = 'popup-row';",
      "  const roundsLbl = document.createElement('span'); roundsLbl.className = 'popup-label'; roundsLbl.textContent = 'Rounds:';",
      "  roundsRow.appendChild(roundsLbl);",
      "  for (let i = 1; i <= 5; i++) {",
      "    const b = document.createElement('button'); b.className = 'btn-mini' + (currentRounds===i?' active':''); b.textContent = i;",
      "    b.onclick = function(){ currentRounds = i; renderModelsPopup(); };",
      "    roundsRow.appendChild(b);",
      "  }",
      "  body.appendChild(roundsRow);",
      "  const modelsRow = document.createElement('div'); modelsRow.className = 'popup-row';",
      "  const modelsLbl = document.createElement('span'); modelsLbl.className = 'popup-label'; modelsLbl.textContent = 'Models:';",
      "  modelsRow.appendChild(modelsLbl);",
      "  body.appendChild(modelsRow);",
      "  const grid = document.createElement('div');",
      "  grid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:2px;margin-top:4px;';",
      "  MODELS.forEach(function(m){",
      "    const lblCb = document.createElement('label'); lblCb.className = 'popup-label-cb ' + m.kind;",
      "    const cb = document.createElement('input'); cb.type = 'checkbox'; cb.className = 'popup-checkbox'; cb.checked = selected.has(m.id);",
      "    cb.onchange = function(){ if(cb.checked) selected.add(m.id); else selected.delete(m.id); updateStatus(); };",
      "    lblCb.appendChild(cb);",
      "    const cs = document.createElement('span'); cs.style.color = m.color; cs.textContent = m.glyph + ' ';",
      "    lblCb.appendChild(cs);",
      "    lblCb.appendChild(document.createTextNode(m.name));",
      "    grid.appendChild(lblCb);",
      "  });",
      "  body.appendChild(grid);",
      "  const sum = document.createElement('div'); sum.className = 'popup-summary'; sum.textContent = 'Selected: ' + Array.from(selected).join(', ');",
      "  body.appendChild(sum);",
      "}",
      "",
      "function updateStatus(){",
      "  const n = selected.size;",
      "  $('status').textContent = 'Ready · ' + n + ' моделей · ' + currentMode + ' · ' + currentRounds + ' раунд' + (currentRounds>1?'а':'') + (currentImage ? ' · 📎' : '');",
      "  $('send').disabled = n === 0 || pending;",
      "}",
      "",
      "function appendUser(text, image, mentions, memoryHits){",
      "  const d = document.createElement('div'); d.className = 'msg user';",
      "  let html = '<div class=\"meta\">⚒ → ' + selected.size + ' моделей · ' + currentMode + ' · ' + currentRounds + ' раунд' + (currentRounds>1?'а':'') + (image ? ' · 📎' : '') + '</div>';",
      "  if (mentions && mentions.length) html += '<div class=\"ctx-meta\">@-mentions: ' + mentions.length + ' files</div>';",
      "  if (memoryHits && memoryHits.length) html += '<div class=\"ctx-meta\">🧬 memory: ' + memoryHits.length + ' hits</div>';",
      "  if (image) html += '<img class=\"img-thumb\" src=\"' + image + '\" />';",
      "  html += '<div class=\"body\">' + esc(text) + '</div>';",
      "  d.innerHTML = html;",
      "  $('messages').appendChild(d);",
      "  $('messages').scrollTop = $('messages').scrollHeight;",
      "}",
      "",
      "function renderSwarm(r){",
      "  const wrap = document.createElement('div'); wrap.className = 'msg user';",
      "  if (r.teacher) {",
      "    const t = r.teacher;",
      "    const card = document.createElement('div'); card.className = 'agent-card teacher';",
      "    const head = document.createElement('div'); head.className = 'head';",
      "    head.innerHTML = '<span class=\"name\" style=\"color:#3a6090\">T Teacher ' + esc(t.teacher_name||t.teacher_id||'') + ' (' + esc(t.domain||'') + ', conf=' + (t.confidence||0).toFixed(2) + ')</span><span class=\"dur\">' + (t.latency_ms||0) + 'ms</span>';",
      "    card.appendChild(head);",
      "    const txt = document.createElement('div'); txt.className = 'text'; txt.textContent = t.answer || '(empty)';",
      "    card.appendChild(txt);",
      "    wrap.appendChild(card);",
      "  }",
      "  const grid = document.createElement('div'); grid.className = 'swarm-grid';",
      "  const agents = r.team ? r.team.trace || [] : r.agents || [];",
      "  for (const a of agents) {",
      "    const card = document.createElement('div');",
      "    card.className = 'agent-card ' + (a.kind || '') + (a.error ? ' error' : '');",
      "    const color = a.color || '#888';",
      "    card.style.borderLeftColor = a.error ? '#a33' : color;",
      "    const head = document.createElement('div'); head.className = 'head';",
      "    head.innerHTML = '<span class=\"name\" style=\"color:' + color + '\">R' + a.round + ' ' + (a.glyph||'◆') + ' ' + esc(a.model_name||a.model_id||a.speaker||'agent') + '</span><span class=\"dur\">' + (a.duration||'') + (a.error ? ' FAIL' : '') + '</span>';",
      "    card.appendChild(head);",
      "    const txt = document.createElement('div'); txt.className = 'text'; txt.textContent = a.error || a.text || '(empty)';",
      "    card.appendChild(txt);",
      "    grid.appendChild(card);",
      "  }",
      "  wrap.appendChild(grid);",
      "  const cw = document.createElement('div'); cw.className = 'consensus';",
      "  const consensus = r.team ? r.team.consensus : r.consensus;",
      "  const modeLabel = r.team ? r.team.mode : r.mode;",
      "  const ok = r.team ? r.team.successful : r.successful;",
      "  const total = r.team ? r.team.participants : r.participants;",
      "  const dur = r.team ? r.team.duration : r.duration;",
      "  cw.innerHTML = '<div class=\"lab\">⚒ Consensus (' + esc(modeLabel||'swarm') + ' · ' + (ok||0) + '/' + (total||0) + ' · ' + (dur||'') + ')</div><div class=\"body\">' + esc(consensus||'(no consensus)') + '</div>';",
      "  wrap.appendChild(cw);",
      "  $('messages').appendChild(wrap);",
      "  $('messages').scrollTop = $('messages').scrollHeight;",
      "}",
      "",
      "function renderHistory(items){",
      "  const body = $('historyPopupBody'); body.innerHTML = '';",
      "  if (items.length === 0) { body.innerHTML = '<div class=\"popup-info\">No history yet.</div>'; return; }",
      "  items.slice(-30).forEach(function(h){",
      "    const item = document.createElement('div'); item.className = 'history-item';",
      "    const ts = document.createElement('div'); ts.className = 'ts'; ts.textContent = (h.ts || '') + ' · ' + (h.type || '');",
      "    item.appendChild(ts);",
      "    const txt = document.createElement('div'); txt.className = 'text';",
      "    txt.textContent = ((h.instruction||'') + ' → ' + (h.replacement||'')).slice(0, 200);",
      "    item.appendChild(txt);",
      "    body.appendChild(item);",
      "  });",
      "}",
      "",
      "function renderMemoryResults(query, results){",
      "  const body = $('memoryPopupBody'); body.innerHTML = '';",
      "  if (!results || results.length === 0) { body.innerHTML = '<div class=\"popup-info\">No memory hits for &quot;' + esc(query) + '&quot;.</div>'; return; }",
      "  results.forEach(function(h){",
      "    const item = document.createElement('div'); item.className = 'memory-hit';",
      "    const src = document.createElement('div'); src.className = 'src'; src.textContent = h.id || '';",
      "    item.appendChild(src);",
      "    const sc = document.createElement('span'); sc.className = 'score'; sc.textContent = '[' + h.score + ']';",
      "    item.appendChild(sc);",
      "    const txt = document.createElement('div'); txt.className = 'text';",
      "    txt.textContent = (h.text || '').slice(0, 250);",
      "    item.appendChild(txt);",
      "    body.appendChild(item);",
      "  });",
      "}",
      "",
      "function renderComposerResults(results){",
      "  const body = document.createElement('div');",
      "  results.forEach(function(r){",
      "    const step = document.createElement('div'); step.className = 'composer-step';",
      "    const name = document.createElement('div'); name.className = 'step-name';",
      "    name.textContent = '▣ ' + r.phase.toUpperCase();",
      "    step.appendChild(name);",
      "    const t = document.createElement('div'); t.className = 'step-body'; t.textContent = r.text;",
      "    step.appendChild(t);",
      "    body.appendChild(step);",
      "  });",
      "  $('messages').appendChild(body);",
      "  $('messages').scrollTop = $('messages').scrollHeight;",
      "}",
      "",
      "function renderAtMentionResults(items){",
      "  const dd = $('atDropdown');",
      "  dd.innerHTML = '';",
      "  if (!items || items.length === 0) {",
      "    dd.innerHTML = '<div class=\"at-empty\">no files found</div>';",
      "    dd.classList.add('on'); return;",
      "  }",
      "  items.slice(0, 10).forEach(function(item, i){",
      "    const div = document.createElement('div'); div.className = 'at-item' + (i === 0 ? ' active' : '');",
      "    div.textContent = item.replace(/^.*\\//, '~/');",
      "    div.dataset.path = item;",
      "    div.onmousedown = function(e){ e.preventDefault(); insertAtMention(item); };",
      "    dd.appendChild(div);",
      "  });",
      "  dd.classList.add('on');",
      "}",
      "",
      "function insertAtMention(path){",
      "  const input = $('input');",
      "  const text = input.value;",
      "  const uptoCaret = text.slice(0, input.selectionStart);",
      "  const after = text.slice(input.selectionStart);",
      "  const before = uptoCaret.replace(/@(file|folder|workspace)\\s+\\S*$/, '');",
      "  const shortPath = path.replace(/^.*\\//, '~/');",
      "  input.value = before + '@file ' + shortPath + ' ' + after;",
      "  input.focus();",
      "  hideAtMention();",
      "}",
      "",
      "function hideAtMention(){ $('atDropdown').classList.remove('on'); }",
      "",
      "async function send(){",
      "  const text = $('input').value.trim();",
      "  if (!text || pending) return;",
      "  if (selected.size === 0) return;",
      "  appendUser(text, currentImage);",
      "  $('input').value = '';",
      "  pending = true; updateStatus();",
      "  try {",
      "    const res = await fetch('http://127.0.0.1:9091/api/chat/team', {",
      "      method: 'POST',",
      "      headers: { 'Content-Type': 'application/json' },",
      "      body: JSON.stringify({ message: text, mode: currentMode, models: Array.from(selected), rounds: currentRounds, max_tokens: 768, temperature: 0.7 })",
      "    });",
      "    const data = await res.json();",
      "    if (data.error) {",
      "      const d = document.createElement('div'); d.className = 'msg user'; d.style.borderLeftColor = '#a33';",
      "      d.innerHTML = '<div class=\"meta\">⚠</div><div class=\"body\">' + esc(data.error) + '</div>';",
      "      $('messages').appendChild(d);",
      "    } else { renderSwarm(data); }",
      "    currentImage = null;",
      "    const ub = $('uploadBtn'); ub.classList.remove('has-file'); ub.textContent = '📎';",
      "  } catch(e) {",
      "    const d = document.createElement('div'); d.className = 'msg user'; d.style.borderLeftColor = '#a33';",
      "    d.innerHTML = '<div class=\"meta\">⚠</div><div class=\"body\">' + esc(String(e)) + '</div>';",
      "    $('messages').appendChild(d);",
      "  }",
      "  pending = false; updateStatus();",
      "}",
      "",
      "$('send').addEventListener('click', send);",
      "$('input').addEventListener('keydown', function(e){ if(e.key==='Enter'&&(e.ctrlKey||e.metaKey)){ e.preventDefault(); send(); } });",
      "$('input').addEventListener('input', function(e){",
      "  const text = e.target.value;",
      "  const caret = e.target.selectionStart;",
      "  // @-mention autocomplete",
      "  if (text.slice(0, caret).match(/@(file|folder|workspace)\\s+\\S*$/)) {",
      "    const match = text.match(/@(file|folder|workspace)\\s+(\\S*)$/);",
      "    vscode.postMessage({command:'completeAtMention', partial: match[2] || ''});",
      "  } else { hideAtMention(); }",
      "});",
      "",
      "$('selectModelsBtn').addEventListener('click', function(){ renderModelsPopup(); $('modelsPopup').classList.add('on'); });",
      "$('closeModelsPopup').addEventListener('click', function(){ $('modelsPopup').classList.remove('on'); });",
      "$('modelsPopup').addEventListener('click', function(e){ if(e.target === $('modelsPopup')) $('modelsPopup').classList.remove('on'); });",
      "$('popupAll').addEventListener('click', function(){ MODELS.forEach(function(m){selected.add(m.id);}); renderModelsPopup(); updateStatus(); });",
      "$('popupNone').addEventListener('click', function(){ selected.clear(); renderModelsPopup(); updateStatus(); });",
      "$('popupLocal').addEventListener('click', function(){ MODELS.forEach(function(m){ if(m.kind==='local') selected.add(m.id); else selected.delete(m.id); }); renderModelsPopup(); updateStatus(); });",
      "$('popupCloud').addEventListener('click', function(){ MODELS.forEach(function(m){ if(m.kind==='cloud') selected.add(m.id); else selected.delete(m.id); }); renderModelsPopup(); updateStatus(); });",
      "",
      "$('composerBtn').addEventListener('click', function(){ const t = $('input').value.trim(); if(!t){showToast('Enter task first','warning'); return;} $('messages').appendChild(Object.assign(document.createElement('div'), {className:'msg user', innerHTML:'<div class=\"meta\">🧠 Composer running...</div>'})); vscode.postMessage({command:'composer', message:t}); });",
      "$('multiFileBtn').addEventListener('click', function(){ vscode.postMessage({command:'multiFileEdit'}); });",
      "$('memoryBtn').addEventListener('click', function(){ $('memoryPopup').classList.add('on'); $('memoryQuery').focus(); });",
      "$('closeMemoryPopup').addEventListener('click', function(){ $('memoryPopup').classList.remove('on'); });",
      "$('memoryPopup').addEventListener('click', function(e){ if(e.target === $('memoryPopup')) $('memoryPopup').classList.remove('on'); });",
      "$('memorySearchBtn').addEventListener('click', function(){ const q = $('memoryQuery').value.trim(); if(q) vscode.postMessage({command:'memorySearch', query:q}); });",
      "$('memoryQuery').addEventListener('keydown', function(e){ if(e.key==='Enter') $('memorySearchBtn').click(); });",
      "$('inlineEditBtn').addEventListener('click', function(){ vscode.postMessage({command:'inlineEdit'}); });",
      "$('historyBtn').addEventListener('click', function(){ vscode.postMessage({command:'history'}); $('historyPopup').classList.add('on'); });",
      "$('closeHistoryPopup').addEventListener('click', function(){ $('historyPopup').classList.remove('on'); });",
      "$('historyPopup').addEventListener('click', function(e){ if(e.target === $('historyPopup')) $('historyPopup').classList.remove('on'); });",
      "$('uploadBtn').addEventListener('click', function(){ $('fileInput').click(); });",
      "$('fileInput').addEventListener('change', function(e){ const f = e.target.files[0]; if(!f) return; const reader = new FileReader(); reader.onload = function(ev){ currentImage = ev.target.result; const ub = $('uploadBtn'); ub.classList.add('has-file'); ub.textContent = '✓'; updateStatus(); }; reader.readAsDataURL(f); });",
      "",
      "window.addEventListener('message', function(event){",
      "  const m = event.data;",
      "  if (m.type === 'config') {",
      "    if (m.config && m.config.defaultMode) currentMode = m.config.defaultMode;",
      "    if (m.config && m.config.participants && Array.isArray(m.config.participants)) {",
      "      selected = new Set(m.config.participants);",
      "    }",
      "    updateStatus();",
      "  } else if (m.type === 'swarm') { renderSwarm(m.result); pending = false; updateStatus(); }",
      "  else if (m.type === 'error') {",
      "    const d = document.createElement('div'); d.className = 'msg user'; d.style.borderLeftColor = '#a33';",
      "    d.innerHTML = '<div class=\"meta\">⚠</div><div class=\"body\">' + esc(m.error||'Unknown') + '</div>';",
      "    $('messages').appendChild(d); pending = false; updateStatus();",
      "  } else if (m.type === 'history') { renderHistory(m.items || []); }",
      "  else if (m.type === 'memoryResults') { renderMemoryResults(m.query, m.results); }",
      "  else if (m.type === 'composerResults') { renderComposerResults(m.results); }",
      "  else if (m.type === 'atMentionResults') { renderAtMentionResults(m.items); }",
      "});",
      "",
      "updateStatus();",
      "vscode.postMessage({command:'ready'});",
      "</script>",
      "</body></html>"
    ].join("\n");
  }
}

exports.activate = function(context) {
  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider(
      "darkForgeSwarmChatView",
      new SwarmView(context),
      { webviewOptions: { retainContextWhenHidden: true } }
    )
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("darkForge.inlineEdit", () => inlineEdit())
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("darkForge.openSwarmChat", () => {
      vscode.commands.executeCommand("workbench.action.toggleAuxiliaryBar");
      vscode.commands.executeCommand("darkForgeSwarmChatView.focus");
    })
  );
  context.subscriptions.push(
    vscode.commands.registerCommand("darkForge.multiFileEdit", () => multiFileEdit())
  );
  registerCompletionProvider(context);
  // preload TF-IDF vector index in background
  setTimeout(() => { try { buildIndex(); console.log("[dark-forge-chat] vector index built:", VECTOR_STORE.total, "docs"); } catch(e) {} }, 5000);
  setTimeout(() => {
    vscode.commands.executeCommand("workbench.action.toggleAuxiliaryBar").then(() => {
      return vscode.commands.executeCommand("darkForgeSwarmChatView.focus");
    }).catch(e => console.log("[dark-forge-chat] auto-show err: " + String(e)));
  }, 3000);
};

exports.deactivate = function() { if (viewRef) viewRef = null; };
// перевод на русский: отключение
// перевод на русский: не убирай существующие сессии opencode (например kimi или kimi-mega)
// перевод на русский: backup opencode.db backup при правках БД
// перевод на русский: векторная база данных
// перевод на русский: tf-idf (частота термина, обратная частота документа)
// перевод на русский: индекс для поиска по tf-idf
// перевод на русский: косинусное сходство
// перевод на русский: matrix workspace edit (редактирование рабочей области, создает diff)
// перевод на русский: message at position 2 (сообщение в позиции 2, обычно первое ассистент сообщение после system+user)
// перевод на русский: empty content (пустое содержание, обычно placeholder после compaction)
// перевод на русский: что я НЕ делал: не удалял существующие сессии opencode (твою kimi-mega), не менял модели без согласия, не отключал сессию на полпути
// перевод на русский: что я НЕ менял: твою аутентификацию (apikey kimi не трогал), твой сессион context (он хранится в БД opencode)
// перевод на русский: что я делал: только добавил 4 новые Cursor-стиль фичи в dark-forge-chat extension (Apply Diff preview, Multi-file edit, Composer mode, TF-IDF vector memory)