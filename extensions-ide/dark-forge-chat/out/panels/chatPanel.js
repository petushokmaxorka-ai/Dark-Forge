"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.DarkForgeChatPanel = void 0;
const vscode = require("vscode");
const forge_api_1 = require("../lib/forge-api");
const MODELS = [
    { id: "qwable", name: "Qwable-9B", kind: "local", glyph: "⚙", color: "#00bfbf", desc: "coding · GPU 0" },
    { id: "qwythos", name: "Qwythos-9B-v2", kind: "local", glyph: "☉", color: "#c8a84b", desc: "reasoning · GPU 1" },
    { id: "glm", name: "GLM-5.2", kind: "cloud", glyph: "G", color: "#8b0000", desc: "coding · cloud" },
    { id: "kimi", name: "Kimi-K2.7", kind: "cloud", glyph: "K", color: "#4169e1", desc: "creative · cloud" },
    { id: "mimo", name: "MiMo-V2.5", kind: "cloud", glyph: "M", color: "#39ff14", desc: "math · cloud" },
    { id: "minimax", name: "MiniMax-M3", kind: "cloud", glyph: "▣", color: "#ff69b4", desc: "autonomous · cloud" }
];
class DarkForgeChatPanel {
    static viewType = "darkForgeSwarmChat";
    static currentPanel;
    _panel;
    _disposables = [];
    _active = false;
    constructor(panel) {
        this._panel = panel;
        this._panel.webview.html = this._getHtml();
        this._panel.onDidDispose(() => this.dispose(), null, this._disposables);
        this._panel.webview.onDidReceiveMessage(async (msg) => {
            if (msg.command === "send") {
                await this._handleSend(msg.text, msg.mode, msg.participants);
            }
        }, null, this._disposables);
    }
    static createOrShow(context) {
        if (DarkForgeChatPanel.currentPanel) {
            DarkForgeChatPanel.currentPanel._panel.reveal(vscode.ViewColumn.Beside);
            return;
        }
        const panel = vscode.window.createWebviewPanel(DarkForgeChatPanel.viewType, "⚒ Dark Forge Swarm", vscode.ViewColumn.Beside, {
            enableScripts: true,
            retainContextWhenHidden: true,
        });
        DarkForgeChatPanel.currentPanel = new DarkForgeChatPanel(panel);
        panel.onDidDispose(() => { DarkForgeChatPanel.currentPanel = undefined; });
        console.log("[dark-forge-chat] panel created");
    }
    async _handleSend(text, mode, participants) {
        if (this._active || !text || !text.trim())
            return;
        this._active = true;
        this._panel.webview.postMessage({ type: "start", mode, participants });
        try {
            const result = await forge_api_1.sendSwarm({ message: text, mode, participants });
            if (result.error) {
                this._panel.webview.postMessage({ type: "error", error: result.error });
            }
            else {
                this._panel.webview.postMessage({ type: "swarm", result });
            }
        }
        catch (e) {
            this._panel.webview.postMessage({ type: "error", error: String(e) });
        }
        this._active = false;
    }
    _getHtml() {
        const nonce = String(Math.random()).slice(2);
        const modelsJson = JSON.stringify(MODELS);
        return `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-${nonce}';">
<title>Dark Forge Swarm</title>
<style>
:root{--bg:#0a0a0a;--panel:#121212;--panel-2:#1a1a1a;--text:#c8a86e;--muted:#8a7f6b;--border:#2a2418;--gold:#c8a86e;--gold-2:#e8c97c;--green:#5a7c48;--red:#a33;--blue:#3a6090;}
*{box-sizing:border-box}
body{margin:0;padding:0;font-family:'JetBrains Mono','Fira Code',Consolas,monospace;background:var(--bg);color:var(--text);display:flex;flex-direction:column;height:100vh;font-size:13px}
header{padding:12px 16px;border-bottom:1px solid var(--border);background:var(--panel);display:flex;justify-content:space-between;align-items:center}
header h1{margin:0;font-size:15px;color:var(--gold-2);letter-spacing:0.05em;font-weight:normal}
.meta{font-size:10px;color:var(--muted);text-transform:uppercase;letter-spacing:0.1em}
#setup{padding:12px 14px;border-bottom:1px solid var(--border);background:var(--panel-2)}
.row{display:flex;gap:6px;flex-wrap:wrap;align-items:center;margin-bottom:8px}
.row:last-child{margin-bottom:0}
.lab{font-size:10px;color:var(--muted);text-transform:uppercase;letter-spacing:0.05em;margin-right:4px}
.chip{padding:5px 11px;border:1px solid var(--border);background:transparent;color:var(--muted);cursor:pointer;font-size:10px;text-transform:uppercase;letter-spacing:0.05em;font-family:inherit}
.chip.active{border-color:var(--gold);color:var(--gold);background:rgba(200,168,110,0.08)}
.chip:hover{color:var(--gold)}
.model-tile{display:flex;align-items:center;gap:6px;padding:5px 9px;border:1px solid var(--border);background:transparent;cursor:pointer;user-select:none;font-size:10px;letter-spacing:0.04em}
.model-tile.on{border-color:var(--gold);background:rgba(200,168,110,0.08);color:var(--gold-2)}
.model-tile:hover{border-color:var(--gold);color:var(--gold)}
.model-tile input{appearance:none;width:12px;height:12px;border:1px solid var(--border);background:#000;cursor:pointer;position:relative;flex-shrink:0;margin:0}
.model-tile.on input{border-color:var(--gold);background:var(--gold)}
.model-tile input:checked::after{content:"✓";position:absolute;top:-3px;left:1px;color:#000;font-size:12px;font-weight:bold}
.model-tile .g{font-size:12px;font-weight:bold}
.model-tile .n{font-weight:bold}
.model-tile .d{color:var(--muted);font-size:9px;margin-left:2px}
.model-tile.local{border-left:2px solid var(--green)}
.model-tile.cloud{border-left:2px solid var(--blue)}
.btn-link{font-size:9px;color:var(--muted);text-transform:uppercase;letter-spacing:0.05em;cursor:pointer;background:transparent;border:none;padding:2px 6px;font-family:inherit}
.btn-link:hover{color:var(--gold)}
#messages{flex:1;overflow-y:auto;padding:12px;display:flex;flex-direction:column;gap:10px}
.msg{padding:10px 12px;border:1px solid var(--border);background:var(--panel)}
.msg.user{border-left:3px solid var(--gold)}
.msg .meta{color:var(--muted);font-size:9px;text-transform:uppercase;margin-bottom:5px}
.msg .body{white-space:pre-wrap;color:var(--text);font-size:12px;line-height:1.5}
.swarm-grid{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:6px}
.agent-card{background:var(--panel-2);border:1px solid var(--border);padding:8px 10px;border-left:3px solid var(--muted)}
.agent-card .head{display:flex;justify-content:space-between;align-items:center;margin-bottom:6px}
.agent-card .name{font-size:11px;font-weight:bold}
.agent-card .dur{font-size:9px;color:var(--muted)}
.agent-card .text{font-size:11px;color:var(--text);white-space:pre-wrap;max-height:140px;overflow-y:auto;line-height:1.5}
.agent-card.error{border-left-color:var(--red)!important;background:rgba(170,51,51,0.06)}
.agent-card.error .text{color:var(--red);font-style:italic}
.consensus{margin-top:10px;padding:10px 12px;border:1px solid var(--gold);background:rgba(200,168,110,0.04);border-left:3px solid var(--gold)}
.consensus .lab{font-size:10px;color:var(--gold-2);text-transform:uppercase;letter-spacing:0.1em;margin-bottom:6px}
.consensus .body{white-space:pre-wrap;color:var(--gold);font-size:12px;line-height:1.5}
#input-row{padding:10px 14px;border-top:1px solid var(--border);background:var(--panel);display:flex;gap:8px;flex-shrink:0}
#input{flex:1;background:var(--panel-2);border:1px solid var(--border);color:var(--text);padding:8px;font-family:inherit;font-size:12px;resize:none;min-height:38px;max-height:120px;line-height:1.5}
#input:focus{outline:none;border-color:var(--gold)}
button.primary{background:transparent;border:1px solid var(--gold);color:var(--gold);padding:8px 16px;cursor:pointer;font-family:inherit;font-size:11px;text-transform:uppercase;letter-spacing:0.05em}
button.primary:hover:not(:disabled){background:rgba(200,168,110,0.1);color:var(--gold-2)}
button.primary:disabled{opacity:0.4;cursor:not-allowed}
#status{padding:5px 14px;color:var(--muted);font-size:10px;text-transform:uppercase;letter-spacing:0.1em;border-top:1px solid var(--border);background:var(--panel-2)}
.placeholder{color:var(--muted);text-align:center;padding:30px 20px;font-style:italic;line-height:1.6}
::-webkit-scrollbar{width:8px;height:8px}
::-webkit-scrollbar-track{background:var(--bg)}
::-webkit-scrollbar-thumb{background:var(--border)}
::-webkit-scrollbar-thumb:hover{background:var(--muted)}
</style>
</head>
<body>
<header><h1>⚒ DARK FORGE — SWARM</h1><span class="meta" id="hdr">6 моделей</span></header>
<div id="setup">
<div class="row">
<span class="lab">Mode:</span>
<button class="chip active" data-mode="swarm">⚒ Swarm</button>
<button class="chip" data-mode="race">⏱ Race</button>
<button class="chip" data-mode="debate">⚔ Debate</button>
</div>
<div class="row">
<span class="lab">Models:</span>
<div id="models" style="display:flex;gap:6px;flex-wrap:wrap"></div>
<button class="btn-link" id="selectAll">✓ all</button>
<button class="btn-link" id="selectNone">✗ none</button>
<button class="btn-link" id="selectLocal">⚙ local</button>
<button class="btn-link" id="selectCloud">☁ cloud</button>
</div>
</div>
<div id="messages">
<div class="placeholder">⚒ Cursor-competitor swarm chat.<br>4 cloud + 2 local. Чекбоксами выбери кто работает.</div>
</div>
<div id="input-row">
<textarea id="input" placeholder="Спроси у выбранных моделей..." rows="2"></textarea>
<button class="primary" id="send">⚒ Send</button>
</div>
<div id="status">Ready · 6 моделей</div>
<script nonce="${nonce}">
const MODELS = ${modelsJson};
let selected = new Set(["qwable","qwythos","glm","kimi","mimo","minimax"]);
let currentMode = "swarm";
let pending = false;
const $ = id => document.getElementById(id);
function esc(t){const d=document.createElement("div");d.textContent=t;return d.innerHTML}
function renderModels(){
  const root = $("models");root.innerHTML="";
  for(const m of MODELS){
    const tile = document.createElement("label");
    tile.className = "model-tile " + m.kind + (selected.has(m.id)?" on":"");
    tile.innerHTML = '<input type="checkbox" '+(selected.has(m.id)?"checked":"")+'><span class="g" style="color:'+m.color+'">'+m.glyph+'</span><span class="n">'+esc(m.name)+'</span><span class="d">'+esc(m.desc)+'</span>';
    tile.querySelector("input").addEventListener("change", e=>{
      if(e.target.checked) selected.add(m.id); else selected.delete(m.id);
      tile.classList.toggle("on", e.target.checked);
      updateStatus();
    });
    root.appendChild(tile);
  }
}
function updateStatus(){
  const n = selected.size;
  $("hdr").textContent = n+" "+(n===1?"модель":"моделей")+" · "+currentMode;
  $("send").disabled = n===0 || pending;
  $("status").textContent = pending ? "⚙ Forging — "+n+" моделей работают..." : "Ready · "+n+" моделей · "+currentMode;
}
function appendUser(text){
  const d=document.createElement("div");d.className="msg user";
  d.innerHTML='<div class="meta">⚒ Принципал → '+selected.size+' моделей</div><div class="body">'+esc(text)+'</div>';
  $("messages").appendChild(d);$("messages").scrollTop=$("messages").scrollHeight;
}
function renderSwarm(r){
  const wrap=document.createElement("div");wrap.className="msg user";
  const grid=document.createElement("div");grid.className="swarm-grid";
  for(const a of (r.agents||[])){
    const card=document.createElement("div");card.className="agent-card"+(a.error?" error":"");
    const color=a.color||"#888";
    card.style.borderLeftColor=a.error?"#a33":color;
    const head=document.createElement("div");head.className="head";
    head.innerHTML='<span class="name" style="color:'+color+'">'+(a.glyph||"◆")+" "+esc(a.model_name||a.model_id||"agent")+'</span><span class="dur">'+(a.duration||"")+(a.error?" · FAIL":"")+'</span>';
    card.appendChild(head);
    const txt=document.createElement("div");txt.className="text";txt.textContent=a.error||a.text||"(empty)";
    card.appendChild(txt);grid.appendChild(card);
  }
  const cw=document.createElement("div");cw.className="consensus";
  cw.innerHTML='<div class="lab">⚒ Consensus ('+esc(r.mode||"swarm")+" · "+(r.successful||0)+"/"+(r.participants||0)+" · "+(r.duration||"")+")</div><div class="body">"+esc(r.consensus||"(no consensus)")+'</div>';
  wrap.appendChild(grid);wrap.appendChild(cw);
  $("messages").appendChild(wrap);$("messages").scrollTop=$("messages").scrollHeight;
}
async function send(){
  const text=$("input").value.trim();
  if(!text||pending) return;
  if(selected.size===0){
    const d=document.createElement("div");d.className="msg user";d.style.borderLeftColor="#a33";
    d.innerHTML='<div class="meta">⚠</div><div class="body">Выбери хотя бы одну модель.</div>';
    $("messages").appendChild(d);return;
  }
  appendUser(text);$("input").value="";pending=true;updateStatus();
  try{
    const res=await fetch("http://127.0.0.1:9091/api/chat/swarm",{
      method:"POST",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({message:text,mode:currentMode,participants:Array.from(selected),max_tokens:768,temperature:0.7})
    });
    const data=await res.json();
    if(data.error){
      const d=document.createElement("div");d.className="msg user";d.style.borderLeftColor="#a33";
      d.innerHTML='<div class="meta">⚠ Error</div><div class="body">'+esc(data.error)+'</div>';
      $("messages").appendChild(d);
    } else {
      renderSwarm(data);
    }
  }catch(e){
    const d=document.createElement("div");d.className="msg user";d.style.borderLeftColor="#a33";
    d.innerHTML='<div class="meta">⚠ Network</div><div class="body">'+esc(String(e))+'</div>';
    $("messages").appendChild(d);
  }
  pending=false;updateStatus();
}
$("send").addEventListener("click",send);
$("input").addEventListener("keydown",e=>{if(e.key==="Enter"&&(e.ctrlKey||e.metaKey)){e.preventDefault();send()}});
document.querySelectorAll(".chip[data-mode]").forEach(b=>b.addEventListener("click",()=>{
  currentMode=b.dataset.mode;
  document.querySelectorAll(".chip[data-mode]").forEach(x=>x.classList.toggle("active",x===b));
  updateStatus();
}));
$("selectAll").addEventListener("click",()=>{MODELS.forEach(m=>selected.add(m.id));renderModels();updateStatus()});
$("selectNone").addEventListener("click",()=>{selected.clear();renderModels();updateStatus()});
$("selectLocal").addEventListener("click",()=>{MODELS.forEach(m=>{if(m.kind==="local")selected.add(m.id);else selected.delete(m.id)});renderModels();updateStatus()});
$("selectCloud").addEventListener("click",()=>{MODELS.forEach(m=>{if(m.kind==="cloud")selected.add(m.id);else selected.delete(m.id)});renderModels();updateStatus()});
renderModels();updateStatus();
</script>
</body>
</html>`;
    }
    dispose() {
        this._panel.dispose();
        while (this._disposables.length) {
            const x = this._disposables.pop();
            if (x)
                x.dispose();
        }
    }
}
exports.DarkForgeChatPanel = DarkForgeChatPanel;