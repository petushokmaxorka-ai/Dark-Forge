import * as vscode from 'vscode'
export function quickAction(action: string) {
  const editor = vscode.window.activeTextEditor
  if (!editor) return
  const selection = editor.document.getText(editor.selection) || editor.document.lineAt(editor.selection.active.line).text
  vscode.window.showInformationMessage(`HereticArch: ${action}`)
}
