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
        traceOutputChannel: window.createOutputChannel('Axon LSP Trace')
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
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    return client.stop();
}