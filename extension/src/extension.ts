import * as vscode from 'vscode'
import { HereticArchChatViewProvider } from './panels/chatPanel'
import { ComposerPanel } from './panels/composerPanel'
import { HereticCompletionProvider } from './providers/inlineCompletionProvider'
import { inlineEdit } from './commands/inlineEdit'
import { quickAction } from './commands/quickAction'

export function activate(context: vscode.ExtensionContext): void {
  const provider = new HereticArchChatViewProvider(context.extensionUri)
  const chatView = vscode.window.registerWebviewViewProvider(
    HereticArchChatViewProvider.viewType, provider,
    { webviewOptions: { retainContextWhenHidden: true } },
  )
  context.subscriptions.push(
    chatView,
    vscode.commands.registerCommand('hereticArch.openChat', () => vscode.commands.executeCommand('hereticArch.chatView.focus')),
    vscode.commands.registerCommand('hereticArch.inlineEdit', () => inlineEdit()),
    vscode.commands.registerCommand('hereticArch.explain', () => quickAction('explain')),
    vscode.commands.registerCommand('hereticArch.fix', () => quickAction('fix')),
    vscode.commands.registerCommand('hereticArch.refactor', () => quickAction('refactor')),
    vscode.commands.registerCommand('hereticArch.test', () => quickAction('test')),
    vscode.commands.registerCommand('hereticArch.openComposer', () => ComposerPanel.createOrShow(context.extensionUri)),
    vscode.commands.registerCommand('hereticArch.toggleAutocomplete', () => {}),
    vscode.languages.registerInlineCompletionItemProvider({ pattern: '**' }, new HereticCompletionProvider()),
  )
}
export function deactivate(): void {}
