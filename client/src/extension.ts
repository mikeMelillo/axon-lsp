import * as path from 'path';
import { workspace, ExtensionContext } from 'vscode';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    TransportKind
} from 'vscode-languageclient/node';

let client: LanguageClient;

export function activate(context: ExtensionContext) {
    // Determine the path to the Python server script
    const serverScript = context.asAbsolutePath(
        path.join('server', 'axon_lsp', 'server.py')
    );

    // Define how to start the Python Language Server.
    // We use the standard output/input streams for JSON-RPC communication.
    const serverOptions: ServerOptions = {
        command: "python3", // Note: In a production extension, you might want to resolve the user's active Python environment.
        args: [serverScript],
        transport: TransportKind.stdio
    };

    // Options to control the language client
    const clientOptions: LanguageClientOptions = {
        // Register the server for Axon documents (.axon, .trio)
        documentSelector: [{ scheme: 'file', language: 'axon' }],
        synchronize: {
            // Notify the server about file changes to '.axon' files contained in the workspace
            fileEvents: workspace.createFileSystemWatcher('**/*.axon')
        }
    };

    // Create the language client and start the client.
    client = new LanguageClient(
        'axonLspClient',
        'Axon Language Server',
        serverOptions,
        clientOptions
    );

    // Start the client. This will also launch the server process.
    client.start();
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    // Ensure we gracefully shut down the language server when VS Code closes
    return client.stop();
}