import * as path from 'path';
import { workspace, ExtensionContext, window } from 'vscode';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    TransportKind
} from 'vscode-languageclient/node';

let client: LanguageClient;

export function activate(context: ExtensionContext) {
    // 1. Create the output channel immediately so it appears in the dropdown
    const outputChannel = window.createOutputChannel('Axon Language Server');
    outputChannel.appendLine('Axon Extension Activating...');

    // 2. Path to the python server script
    const serverScript = context.asAbsolutePath(
        path.join('server', 'axon_lsp', 'server.py')
    );

    const pythonCommand = process.platform === 'win32' ? 'python' : 'python3';

    // 3. Server options: how to launch the python process
    const serverOptions: ServerOptions = {
        command: pythonCommand,
        args: [serverScript],
        transport: TransportKind.stdio,
        options: {
            env: {
                ...process.env,
                PYTHONUNBUFFERED: "1" // Ensures logs aren't delayed by python's buffer
            }
        }
    };

    // Helper function to get current settings
    function getSettings() {
        const config = workspace.getConfiguration('axonLsp');
        return {
            haxallPaths: config.get<string[]>('haxallPaths', []),
            externalPaths: config.get<string[]>('externalPaths', [])
        };
    }

    // 4. Client options: which files to watch and where to log
    const clientOptions: LanguageClientOptions = {
        // Must match the language ID in package.json
        documentSelector: [
            { scheme: 'file', language: 'axon' }
        ],
        synchronize: {
            // Notify the server about file changes in the workspace
            fileEvents: workspace.createFileSystemWatcher('**/{*.axon,*.trio}')
        },
        outputChannel: outputChannel,
        traceOutputChannel: window.createOutputChannel('Axon LSP Trace'),
        initializationOptions: getSettings()
    };

    // 5. Create and start the client
    client = new LanguageClient(
        'axonLspClient',
        'Axon Language Server',
        serverOptions,
        clientOptions
    );

    outputChannel.appendLine('Starting Language Client...');
    client.start().catch(err => {
        outputChannel.appendLine(`Failed to start client: ${err}`);
    });

    // 6. Listen for settings changes and notify server
    context.subscriptions.push(
        workspace.onDidChangeConfiguration(() => {
            outputChannel.appendLine('Settings changed, reloading...');
            const settings = getSettings();
            client.sendNotification('axon/reloadSettings', settings).then(() => {
                outputChannel.appendLine('Settings reloaded successfully');
            }).catch((err) => {
                outputChannel.appendLine(`Failed to reload settings: ${err}`);
            });
        })
    );
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    return client.stop();
}