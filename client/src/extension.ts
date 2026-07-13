import * as path from 'path';
import * as fs from 'fs';
import { workspace, ExtensionContext, window, commands, env, Uri, Location } from 'vscode';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    TransportKind,
    ProvideDefinitionSignature
} from 'vscode-languageclient/node';

let client: LanguageClient;

export function activate(context: ExtensionContext) {
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
            { scheme: 'file', language: 'axon' }
        ],
        synchronize: {
            // Notify the server about file changes in the workspace
            fileEvents: workspace.createFileSystemWatcher('**/{*.axon,*.trio}')
        },
        outputChannel: outputChannel,
        traceOutputChannel: window.createOutputChannel('Axon LSP Trace'),
        middleware: {
            provideDefinition: async (document, position, token, next: ProvideDefinitionSignature) => {
                const result = await next(document, position, token);
                const locations = Array.isArray(result) ? result : result ? [result] : [];
                const external = locations.find((item) => item instanceof Location && item.uri.scheme.startsWith('http'));
                if (external instanceof Location) {
                    await env.openExternal(external.uri);
                    return null;
                }
                return result;
            }
        }
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

    // 6. Register command to open external URLs in browser
    context.subscriptions.push(
        commands.registerCommand('extension.openExternal', (url: string) => {
            console.log('OpenExternal called with:', url); 
            env.openExternal(Uri.parse(url));
        })
    );
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
