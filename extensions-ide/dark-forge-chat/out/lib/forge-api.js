"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.getConfig = getConfig;
exports.sendSwarm = sendSwarm;
function getConfig() {
    const cfg = vscode.workspace.getConfiguration("darkForge");
    return {
        baseUrl: cfg.get("baseUrl") || "http://127.0.0.1:9091",
        defaultMode: cfg.get("defaultMode") || "swarm",
        participants: cfg.get("participants") || ["qwable", "qwythos", "glm", "kimi", "mimo", "minimax"]
    };
}
const vscode = require("vscode");
async function sendSwarm(payload) {
    const cfg = getConfig();
    const res = await fetch(`${cfg.baseUrl}/api/chat/swarm`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            message: payload.message,
            mode: payload.mode || cfg.defaultMode,
            participants: payload.participants || cfg.participants,
            max_tokens: payload.max_tokens || 768,
            temperature: payload.temperature || 0.7,
        }),
    });
    if (!res.ok) return { error: `HTTP ${res.status}: ${await res.text()}` };
    return await res.json();
}