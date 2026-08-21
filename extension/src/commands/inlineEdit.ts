import * as vscode from 'vscode'
export async function inlineEdit() {
  const editor = vscode.window.activeTextEditor
  if (!editor) return
  const input = await vscode.window.showInputBox({ prompt: 'Inline edit instruction', placeHolder: 'What to change?' })
  if (!input) return
  vscode.window.showInformationMessage('Inline edit: ' + input)
}
