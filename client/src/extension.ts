import * as path from 'path';
import * as fs from 'fs';
import { workspace, ExtensionContext, ExtensionMode, window, commands, env, Uri, TextDocument } from 'vscode';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    TransportKind
} from 'vscode-languageclient/node';

let client: LanguageClient | undefined;

export function activate(context: ExtensionContext) {
    // The development host should be useful immediately, even when VS Code
    // restores an unrelated editor instead of an Axon document. In normal use
    // keep the language server lazy and start it when the first Axon-family
    // document is opened.
    if (context.extensionMode !== ExtensionMode.Development && !hasAxonDocumentOpen()) {
        const listener = workspace.onDidOpenTextDocument(document => {
            if (!isAxonDocument(document)) {
                return;
            }
            listener.dispose();
            startLanguageClient(context);
        });
        context.subscriptions.push(listener);
        return;
    }

    startLanguageClient(context);
}

function hasAxonDocumentOpen(): boolean {
    return workspace.textDocuments.some(isAxonDocument)
        || (window.activeTextEditor !== undefined && isAxonDocument(window.activeTextEditor.document));
}

function isAxonDocument(document: TextDocument): boolean {
    return ['axon', 'xeto', 'fantom'].includes(document.languageId)
        || /\.(axon|trio|xeto|fan)$/i.test(document.uri.path);
}

function startLanguageClient(context: ExtensionContext): void {
    if (client) {
        return;
    }

    // 1. Create the output channel immediately so it appears in the dropdown
    const outputChannel = window.createOutputChannel('Axon Language Server');
    outputChannel.appendLine('Axon Extension Activating...');

    let serverBinary: string;
    try {
        serverBinary = resolveServerBinary(context);
    } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        outputChannel.appendLine(`Failed to resolve server binary: ${message}`);
        void window.showErrorMessage(`Axon LSP failed to start: ${message}`);
        return;
    }
    outputChannel.appendLine(`Using server binary: ${serverBinary}`);
    let settings = getServerSettings();
    logWorkspaceIndexWarning(outputChannel, settings.indexAllWorkspaceFolders);

    // 3. Server options: how to launch the Go process
    const serverOptions: ServerOptions = {
        command: serverBinary,
        args: [],
        transport: TransportKind.stdio,
        options: { env: { ...process.env } }
    };

    // 4. Client options: which files to watch and where to log
    const clientOptions: LanguageClientOptions = {
        // Must match the language ID in package.json
        documentSelector: [
            { scheme: 'file', language: 'axon' },
            { scheme: 'file', language: 'xeto' },
            { scheme: 'file', language: 'fantom' },
            { scheme: 'file', pattern: '**/*.fan' },
            { scheme: 'file', pattern: '**/*.xeto' }
        ],
        initializationOptions: {
            settings
        },
        synchronize: {
            // Notify the server about file changes in the workspace
            fileEvents: workspace.createFileSystemWatcher('**/{*.axon,*.trio,*.fan,*.xeto}')
        },
        outputChannel: outputChannel,
        traceOutputChannel: window.createOutputChannel('Axon LSP Trace'),
        // Definition requests must remain side-effect free. VS Code can issue
        // them for Ctrl-hover as well as explicit navigation commands.
    };

    // 5. Create and start the client
    const languageClient = new LanguageClient(
        'axonLspClient',
        'Axon Language Server',
        serverOptions,
        clientOptions
    );
    client = languageClient;
    outputChannel.appendLine('Starting Language Client...');
    const clientReady = languageClient.start();
    context.subscriptions.push(workspace.registerTextDocumentContentProvider('axon-ext', {
        provideTextDocumentContent: async uri => {
            await clientReady;
            return languageClient.sendRequest<string>('axonLsp/embeddedDocument', { uri: uri.toString() });
        }
    }));

    clientReady.catch(err => {
        outputChannel.appendLine(`Failed to start client: ${err}`);
    });

    context.subscriptions.push(workspace.onDidChangeConfiguration(event => {
        if (!event.affectsConfiguration('axonLsp')) {
            return;
        }
        const previousSettings = settings;
        settings = getServerSettings();
        if (!previousSettings.indexAllWorkspaceFolders && settings.indexAllWorkspaceFolders) {
            logWorkspaceIndexWarning(outputChannel, true);
        }
        void languageClient.sendNotification('workspace/didChangeConfiguration', {
            settings
        });
    }));
    context.subscriptions.push(workspace.onDidChangeWorkspaceFolders(() => {
        logWorkspaceIndexWarning(outputChannel, settings.indexAllWorkspaceFolders);
    }));

    // 6. Register command to open external URLs in browser
    context.subscriptions.push(
        commands.registerCommand('extension.openExternal', (url: string) => {
            console.log('OpenExternal called with:', url); 
            env.openExternal(Uri.parse(url));
        })
    );
}

interface ServerSettings {
    haxallPaths: string[];
    externalPaths: string[];
    indexAllWorkspaceFolders: boolean;
    mode: string;
    warnMissingArguments: boolean;
}

function getServerSettings(): ServerSettings {
    const config = workspace.getConfiguration('axonLsp');
    const workspaceRoot = workspace.workspaceFolders?.[0]?.uri.fsPath;
    return {
        haxallPaths: normalizeConfiguredPaths(config.get<string[]>('haxallPaths', []), workspaceRoot),
        externalPaths: normalizeConfiguredPaths(config.get<string[]>('externalPaths', []), workspaceRoot),
        indexAllWorkspaceFolders: config.get<boolean>('indexAllWorkspaceFolders', false),
        mode: config.get<string>('mode', 'auto'),
        warnMissingArguments: config.get<boolean>('warnMissingArguments', true)
    };
}

function logWorkspaceIndexWarning(outputChannel: { appendLine(value: string): void }, enabled: boolean): void {
    const count = workspace.workspaceFolders?.length ?? 0;
    if (enabled && count > 1) {
        outputChannel.appendLine(
            `Warning: indexing all ${count} workspace folders as local Axon sources; this may increase startup time and memory usage.`
        );
    }
}

function normalizeConfiguredPaths(paths: string[], workspaceRoot?: string): string[] {
    return paths
        .map(value => value.trim())
        .filter(Boolean)
        .map(value => {
            if (path.isAbsolute(value) || !workspaceRoot) {
                return value;
            }
            return path.resolve(workspaceRoot, value);
        });
}

function resolveServerBinary(context: ExtensionContext): string {
    const override = process.env.AXON_LSP_SERVER_PATH;
    if (override && fs.existsSync(override)) {
        return override;
    }

    const archMap: Record<string, string> = {
        x64: 'x64',
        arm64: 'arm64'
    };

    const platformMap: Record<string, string> = {
        linux: 'linux',
        darwin: 'darwin',
        win32: 'win32'
    };

    const arch = archMap[process.arch];
    const platform = platformMap[process.platform];
    if (!arch || !platform) {
        throw new Error(`Unsupported platform: ${process.platform}/${process.arch}`);
    }

    const executable = process.platform === 'win32'
        ? `axon-lsp-${platform}-${arch}.exe`
        : `axon-lsp-${platform}-${arch}`;
    const binaryPath = context.asAbsolutePath(path.join('bin', executable));
    if (!fs.existsSync(binaryPath)) {
        throw new Error(`server binary not found for ${process.platform}/${process.arch}: ${binaryPath}`);
    }
    return binaryPath;
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    return client.stop();
}
