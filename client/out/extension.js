"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
exports.activate = activate;
exports.deactivate = deactivate;
const path = __importStar(require("path"));
const fs = __importStar(require("fs"));
const vscode_1 = require("vscode");
const node_1 = require("vscode-languageclient/node");
let client;
function activate(context) {
    // 1. Create the output channel immediately so it appears in the dropdown
    const outputChannel = vscode_1.window.createOutputChannel('Axon Language Server');
    outputChannel.appendLine('Axon Extension Activating...');
    let serverBinary;
    try {
        serverBinary = resolveServerBinary(context);
    }
    catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        outputChannel.appendLine(`Failed to resolve server binary: ${message}`);
        void vscode_1.window.showErrorMessage(`Axon LSP failed to start: ${message}`);
        return;
    }
    outputChannel.appendLine(`Using server binary: ${serverBinary}`);
    let settings = getServerSettings();
    logWorkspaceIndexWarning(outputChannel, settings.indexAllWorkspaceFolders);
    // 3. Server options: how to launch the Go process
    const serverOptions = {
        command: serverBinary,
        args: [],
        transport: node_1.TransportKind.stdio,
        options: { env: { ...process.env } }
    };
    // 4. Client options: which files to watch and where to log
    const clientOptions = {
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
            fileEvents: vscode_1.workspace.createFileSystemWatcher('**/{*.axon,*.trio,*.fan,*.xeto}')
        },
        outputChannel: outputChannel,
        traceOutputChannel: vscode_1.window.createOutputChannel('Axon LSP Trace'),
        middleware: {
            provideDefinition: async (document, position, token, next) => {
                const result = await next(document, position, token);
                const locations = Array.isArray(result) ? result : result ? [result] : [];
                const external = locations.find((item) => item instanceof vscode_1.Location && item.uri.scheme.startsWith('http'));
                if (external instanceof vscode_1.Location) {
                    await vscode_1.env.openExternal(external.uri);
                    return null;
                }
                return result;
            }
        }
    };
    // 5. Create and start the client
    client = new node_1.LanguageClient('axonLspClient', 'Axon Language Server', serverOptions, clientOptions);
    outputChannel.appendLine('Starting Language Client...');
    const clientReady = client.start();
    context.subscriptions.push(vscode_1.workspace.registerTextDocumentContentProvider('axon-ext', {
        provideTextDocumentContent: async (uri) => {
            await clientReady;
            return client.sendRequest('axonLsp/embeddedDocument', { uri: uri.toString() });
        }
    }));
    clientReady.catch(err => {
        outputChannel.appendLine(`Failed to start client: ${err}`);
    });
    context.subscriptions.push(vscode_1.workspace.onDidChangeConfiguration(event => {
        if (!event.affectsConfiguration('axonLsp')) {
            return;
        }
        const previousSettings = settings;
        settings = getServerSettings();
        if (!previousSettings.indexAllWorkspaceFolders && settings.indexAllWorkspaceFolders) {
            logWorkspaceIndexWarning(outputChannel, true);
        }
        void client.sendNotification('workspace/didChangeConfiguration', {
            settings
        });
    }));
    context.subscriptions.push(vscode_1.workspace.onDidChangeWorkspaceFolders(() => {
        logWorkspaceIndexWarning(outputChannel, settings.indexAllWorkspaceFolders);
    }));
    // 6. Register command to open external URLs in browser
    context.subscriptions.push(vscode_1.commands.registerCommand('extension.openExternal', (url) => {
        console.log('OpenExternal called with:', url);
        vscode_1.env.openExternal(vscode_1.Uri.parse(url));
    }));
}
function getServerSettings() {
    const config = vscode_1.workspace.getConfiguration('axonLsp');
    const workspaceRoot = vscode_1.workspace.workspaceFolders?.[0]?.uri.fsPath;
    return {
        haxallPaths: normalizeConfiguredPaths(config.get('haxallPaths', []), workspaceRoot),
        externalPaths: normalizeConfiguredPaths(config.get('externalPaths', []), workspaceRoot),
        indexAllWorkspaceFolders: config.get('indexAllWorkspaceFolders', false),
        mode: config.get('mode', 'auto')
    };
}
function logWorkspaceIndexWarning(outputChannel, enabled) {
    const count = vscode_1.workspace.workspaceFolders?.length ?? 0;
    if (enabled && count > 1) {
        outputChannel.appendLine(`Warning: indexing all ${count} workspace folders as local Axon sources; this may increase startup time and memory usage.`);
    }
}
function normalizeConfiguredPaths(paths, workspaceRoot) {
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
function resolveServerBinary(context) {
    const override = process.env.AXON_LSP_SERVER_PATH;
    if (override && fs.existsSync(override)) {
        return override;
    }
    const archMap = {
        x64: 'x64',
        arm64: 'arm64'
    };
    const platformMap = {
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
function deactivate() {
    if (!client) {
        return undefined;
    }
    return client.stop();
}
//# sourceMappingURL=extension.js.map