import * as vscode from 'vscode'
export class HereticCompletionProvider implements vscode.InlineCompletionItemProvider {
  async provideInlineCompletionItems(): Promise<vscode.InlineCompletionItem[]|undefined> { return undefined }
}
