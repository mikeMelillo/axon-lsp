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
const vscode_1 = require("vscode");
const node_1 = require("vscode-languageclient/node");
let client;
function activate(context) {
    // 1. Create the output channel immediately so it appears in the dropdown
    const outputChannel = vscode_1.window.createOutputChannel('Axon Language Server');
    outputChannel.appendLine('Axon Extension Activating...');
    // 2. Path to the python server script
    const serverScript = context.asAbsolutePath(path.join('server', 'axon_lsp', 'server.py'));
    const pythonCommand = process.platform === 'win32' ? 'python' : 'python3';
    // 3. Server options: how to launch the python process
    const serverOptions = {
        command: pythonCommand,
        args: [serverScript],
        transport: node_1.TransportKind.stdio,
        options: {
            env: {
                ...process.env,
                PYTHONUNBUFFERED: "1" // Ensures logs aren't delayed by python's buffer
            }
        }
    };
    // 4. Client options: which files to watch and where to log
    const clientOptions = {
        // Must match the language ID in package.json
        documentSelector: [
            { scheme: 'file', language: 'axon' }
        ],
        synchronize: {
            // Notify the server about file changes in the workspace
            fileEvents: vscode_1.workspace.createFileSystemWatcher('**/{*.axon,*.trio}')
        },
        outputChannel: outputChannel,
        traceOutputChannel: vscode_1.window.createOutputChannel('Axon LSP Trace')
    };
    // 5. Create and start the client
    client = new node_1.LanguageClient('axonLspClient', 'Axon Language Server', serverOptions, clientOptions);
    outputChannel.appendLine('Starting Language Client...');
    client.start().catch(err => {
        outputChannel.appendLine(`Failed to start client: ${err}`);
    });
    // 6. Register command to open external URLs in browser
    context.subscriptions.push(vscode_1.commands.registerCommand('extension.openExternal', (url) => {
        console.log('OpenExternal called with:', url);
        vscode_1.env.openExternal(vscode_1.Uri.parse(url));
    }));
}
function deactivate() {
    if (!client) {
        return undefined;
    }
    return client.stop();
}
//# sourceMappingURL=extension.js.map