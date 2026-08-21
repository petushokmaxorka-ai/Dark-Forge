import * as vscode from 'vscode'
import { forgeBaseUrl } from '../lib/forge-api'

// ═══════════════════════════════════════════════════════════
// Swarm Chat — embeds the Forge backend web UI directly
// No extension webview tricks — just load the real UI
// ═══════════════════════════════════════════════════════════

export class HereticArchChatViewProvider implements vscode.WebviewViewProvider {
  public static readonly viewType = 'hereticArch.chatView'
  private _view?: vscode.WebviewView

  constructor(private readonly _extensionUri: vscode.Uri) {}

  public resolveWebviewView(
    webviewView: vscode.WebviewView,
    _context: vscode.WebviewViewResolveContext,
    _token: vscode.CancellationToken,
  ): void {
    this._view = webviewView
    webviewView.webview.options = {
      enableScripts: true,
      enableForms: true,
    }
    webviewView.webview.html = this._getHtml()
  }

  private _getHtml(): string {
    const FORGE_URL = forgeBaseUrl()
    return `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; frame-src ${FORGE_URL}; connect-src ${FORGE_URL}; style-src 'unsafe-inline'; script-src 'unsafe-inline';">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { 
    background: #0a0a0a; 
    overflow: hidden;
    font-family: 'JetBrains Mono', monospace;
  }
  #loading {
    position: fixed; top: 0; left: 0; right: 0; bottom: 0;
    display: flex; align-items: center; justify-content: center;
    color: #c8a86e; font-size: 14px;
    background: #0a0a0a;
    flex-direction: column; gap: 12px;
    z-index: 10;
  }
  #loading .sigil { font-size: 28px; color: #e8c87e; }
  #loading .text { letter-spacing: 2px; text-transform: uppercase; }
  #loading .sub { color: #5a4a3a; font-size: 10px; }
  #error {
    position: fixed; bottom: 8px; left: 8px; right: 8px;
    background: #2a1010; color: #ff6b6b; padding: 8px;
    font-size: 11px; border-radius: 4px; display: none;
    z-index: 20;
  }
  iframe {
    width: 100%; height: 100vh; border: none;
    background: #0a0a0a;
  }
  .hidden { display: none; }
</style>
</head>
<body>
  <div id="loading">
    <div class="sigil">⚒</div>
    <div class="text">SWARM</div>
    <div class="sub">Connecting to Forge...</div>
  </div>
  <div id="error"></div>
  <iframe id="forge-frame" src="${FORGE_URL}" class="hidden"></iframe>
  <script>
    (function(){
      var frame = document.getElementById('forge-frame');
      var loading = document.getElementById('loading');
      var errorBox = document.getElementById('error');
      var loaded = false;
      frame.addEventListener('load', function(){
        loaded = true;
        loading.classList.add('hidden');
        frame.classList.remove('hidden');
      });
      // Fallback: probe Forge via fetch, then force-load iframe
      fetch('${FORGE_URL}/', { mode: 'no-cors' })
        .then(function(){ frame.src = '${FORGE_URL}/'; })
        .catch(function(e){
          errorBox.textContent = 'Forge ' + '${FORGE_URL}' + ' unreachable: ' + e.message;
          errorBox.style.display = 'block';
          loading.querySelector('.sub').textContent = 'Forge offline — check heretic-forge.service';
        });
      // Timeout: if iframe not loaded in 8s, show error
      setTimeout(function(){
        if (!loaded) {
          loading.querySelector('.sub').textContent = 'Forge slow/unreachable — check :9091';
        }
      }, 8000);
    })();
  </script>
</body>
</html>`
  }
}

// Composer stub
export class HereticArchPanel {
  public static readonly viewType = 'hereticArchChat'
  public static render(): void {}
}
