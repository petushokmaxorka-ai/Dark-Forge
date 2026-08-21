import * as vscode from 'vscode'
export class ComposerPanel {
  public static readonly viewType = 'hereticArch.composer'
  public static createOrShow(_extensionUri: vscode.Uri): void {
    vscode.window.showInformationMessage('Composer coming soon')
  }
}
