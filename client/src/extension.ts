import * as path from 'path';
import * as fs from 'fs';
import { workspace, ExtensionContext, ExtensionMode, window, commands, env, Uri, TextDocument, EventEmitter, RelativePattern, FileSystemWatcher } from 'vscode';
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
    const sourcePattern = '**/*.{trio,axon,fan,xeto,xetolib}';
    const workspaceWatcher = workspace.createFileSystemWatcher(sourcePattern);
    context.subscriptions.push(workspaceWatcher);
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
            fileEvents: workspaceWatcher
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
    const sourceChanges = new EventEmitter<Uri>();
    context.subscriptions.push(sourceChanges);
    languageClient.onNotification('axonLsp/archiveSourcesChanged', () => {
        for (const document of workspace.textDocuments) {
            if (document.uri.scheme === 'axon-ext' && document.uri.path.startsWith('/xetolib/')) {
                sourceChanges.fire(document.uri);
            }
        }
    });
    const clientReady = languageClient.start();
    context.subscriptions.push(workspace.registerTextDocumentContentProvider('axon-ext', {
        onDidChange: sourceChanges.event,
        provideTextDocumentContent: async uri => {
            await clientReady;
            try {
                return await languageClient.sendRequest<string>('axonLsp/embeddedDocument', { uri: uri.toString() });
            } catch (err) {
                if (uri.path.startsWith('/xetolib/') && err instanceof Error && 'code' in err && err.code === -32602) {
                    return '// Archive source is no longer available. It may have been removed or replaced.';
                }
                throw err;
            }
        }
    }));

    clientReady.catch(err => {
        outputChannel.appendLine(`Failed to start client: ${err}`);
    });

    let externalWatchers: { dispose(): void }[] = [];
    let reconcileTimer: ReturnType<typeof setTimeout> | undefined;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let reconcileStarted = 0;
    let reconcilePaths = new Map<string, boolean>();
    let reconcileActive = false;
    let watcherGeneration = 0;
    let watchersDisposed = false;
    function flushReconcile(): void {
        reconcileTimer = undefined;
        if (watchersDisposed || reconcileActive || !reconcilePaths.size) {
            return;
        }
        const batch = reconcilePaths;
        reconcilePaths = new Map();
        reconcileStarted = 0;
        reconcileActive = true;
        const generation = watcherGeneration;
        const retries = [...batch].filter(([, retry]) => retry).map(([uri]) => uri);
        const send = async (attempt: number): Promise<void> => {
            try {
                await clientReady;
                if (watchersDisposed || generation !== watcherGeneration) {
                    return;
                }
                await languageClient.sendNotification('axonLsp/reconcileSources', {
                    uris: attempt === 0 ? [...batch.keys()] : retries
                });
            } catch (err) {
                outputChannel.appendLine(`Failed to reconcile sources: ${err}`);
            }
            if (watchersDisposed || generation !== watcherGeneration) {
                return;
            }
            // A detached batch has at most two retries; fresh events cannot reset it.
            if (retries.length && attempt < 2) {
                retryTimer = setTimeout(() => {
                    retryTimer = undefined;
                    void send(attempt + 1);
                }, attempt === 0 ? 750 : 1500);
            } else {
                reconcileActive = false;
                if (!reconcileTimer) {
                    flushReconcile();
                }
            }
        };
        void send(0);
    }
    function scheduleReconcile(uri: Uri, retry: boolean): void {
        const key = uri.toString();
        reconcilePaths.set(key, retry || reconcilePaths.get(key) === true);
        if (!reconcileStarted) {
            reconcileStarted = Date.now();
        }
        if (reconcileTimer) {
            clearTimeout(reconcileTimer);
        }
        reconcileTimer = setTimeout(flushReconcile, Math.max(0, Math.min(250, 1000 - (Date.now() - reconcileStarted))));
    }
    function updateExternalWatchers(): void {
        externalWatchers.forEach(watcher => watcher.dispose());
        externalWatchers = [];
        watcherGeneration++;
        reconcileActive = false;
        if (reconcileTimer) {
            clearTimeout(reconcileTimer);
            reconcileTimer = undefined;
        }
        if (retryTimer) {
            clearTimeout(retryTimer);
            retryTimer = undefined;
        }
        reconcilePaths.clear();
        reconcileStarted = 0;
        const folders = workspace.workspaceFolders ?? [];
        const selected = settings.indexAllWorkspaceFolders ? folders : folders.slice(0, 1);
        const roots = [...new Set([...selected.map(folder => folder.uri.fsPath), ...settings.externalPaths].map(root => path.resolve(root)))];
        for (const root of roots) {
            // Anchor one level above the nearest existing parent to observe its
            // deletion too. Higher ancestor deletion is outside this boundary.
            let base = path.dirname(root);
            while (!fs.existsSync(base) && path.dirname(base) !== base) {
                base = path.dirname(base);
            }
            base = path.dirname(base);
            const segments = path.relative(base, root).split(path.sep).filter(Boolean)
                .map(segment => segment.replace(/[\[\]*?{}]/g, character => `[${character}]`));
            const prefixes = segments.map((_, i) => segments.slice(0, i + 1).join('/'));
            const rootPattern = prefixes[prefixes.length - 1] ?? '';
            const children = Array.from({ length: 5 }, (_, i) =>
                [rootPattern, ...Array<string>(i + 1).fill('*')].filter(Boolean).join('/'));
            const pattern = `{${[...prefixes, ...children].join(',')}}`;
            const watcher: FileSystemWatcher = workspace.createFileSystemWatcher(new RelativePattern(Uri.file(base), pattern));
            externalWatchers.push(watcher);
            const relevant = (uri: Uri) => {
                const relative = path.relative(root, uri.fsPath);
                const ancestor = path.relative(uri.fsPath, root);
                const inside = relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative));
                const parent = ancestor === '' || (!ancestor.startsWith(`..${path.sep}`) && ancestor !== '..' && !path.isAbsolute(ancestor));
                return parent || (inside && relative.split(path.sep).length <= 5);
            };
            const notify = (uri: Uri, type: number) => {
                if (!relevant(uri)) {
                    return;
                }
                const source = /\.(trio|axon|fan|xeto|xetolib)$/i.test(uri.fsPath);
                let directory = false;
                if (type === 1) {
                    try {
                        directory = fs.statSync(uri.fsPath).isDirectory();
                    } catch {
                        // A create/delete race is handled by the deletion event.
                    }
                }
                const archive = /\.xetolib$/i.test(uri.fsPath);
                if (directory || archive) {
                    scheduleReconcile(uri, true);
                } else if (type === 3) {
                    // Eligibility is checked cheaply on the server, including
                    // directories named like sources. Unknown files get no retries.
                    scheduleReconcile(uri, false);
                }
                // Workspace sources are already synchronized by workspaceWatcher.
                if (directory || !source || folders.some(folder => {
                    const relative = path.relative(folder.uri.fsPath, uri.fsPath);
                    return relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative));
                })) {
                    return;
                }
                const generation = watcherGeneration;
                void clientReady.then(() => {
                    if (!watchersDisposed && generation === watcherGeneration) {
                        return languageClient.sendNotification('workspace/didChangeWatchedFiles', {
                            changes: [{ uri: uri.toString(), type }]
                        });
                    }
                }).catch(err => outputChannel.appendLine(`Failed to notify external source change: ${err}`));
            };
            externalWatchers.push(watcher.onDidCreate(uri => notify(uri, 1)), watcher.onDidChange(uri => notify(uri, 2)), watcher.onDidDelete(uri => notify(uri, 3)));
        }
    }
    updateExternalWatchers();
    context.subscriptions.push({ dispose: () => {
        watchersDisposed = true;
        if (reconcileTimer) {
            clearTimeout(reconcileTimer);
        }
        if (retryTimer) {
            clearTimeout(retryTimer);
        }
        reconcilePaths.clear();
        externalWatchers.forEach(watcher => watcher.dispose());
    } });

    context.subscriptions.push(workspace.onDidChangeConfiguration(event => {
        if (!event.affectsConfiguration('axonLsp')) {
            return;
        }
        const previousSettings = settings;
        settings = getServerSettings();
        updateExternalWatchers();
        if (!previousSettings.indexAllWorkspaceFolders && settings.indexAllWorkspaceFolders) {
            logWorkspaceIndexWarning(outputChannel, true);
        }
        void languageClient.sendNotification('workspace/didChangeConfiguration', {
            settings
        });
    }));
    context.subscriptions.push(workspace.onDidChangeWorkspaceFolders(() => {
        settings = getServerSettings();
        updateExternalWatchers();
        void languageClient.sendNotification('workspace/didChangeConfiguration', { settings });
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
    externalPaths: string[];
    indexAllWorkspaceFolders: boolean;
    mode: string;
    ignoreBundledXetoCache: boolean;
    warnMissingArguments: boolean;
}

function getServerSettings(): ServerSettings {
    const config = workspace.getConfiguration('axonLsp');
    const workspaceRoot = workspace.workspaceFolders?.[0]?.uri.fsPath;
    return {
        externalPaths: normalizeConfiguredPaths(config.get<string[]>('externalPaths', []), workspaceRoot),
        indexAllWorkspaceFolders: config.get<boolean>('indexAllWorkspaceFolders', false),
        mode: config.get<string>('mode', 'auto'),
        ignoreBundledXetoCache: config.get<boolean>('ignoreBundledXetoCache', false),
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
