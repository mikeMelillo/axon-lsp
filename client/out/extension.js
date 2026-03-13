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
    // Determine the path to the Python server script
    const serverScript = context.asAbsolutePath(path.join('server', 'axon_lsp', 'server.py'));
    // Define how to start the Python Language Server.
    // We use the standard output/input streams for JSON-RPC communication.
    const serverOptions = {
        command: "python3", // Note: In a production extension, you might want to resolve the user's active Python environment.
        args: [serverScript],
        transport: node_1.TransportKind.stdio
    };
    // Options to control the language client
    const clientOptions = {
        // Register the server for Axon documents (.axon, .trio)
        documentSelector: [{ scheme: 'file', language: 'axon' }],
        synchronize: {
            // Notify the server about file changes to '.axon' files contained in the workspace
            fileEvents: vscode_1.workspace.createFileSystemWatcher('**/*.axon')
        }
    };
    // Create the language client and start the client.
    client = new node_1.LanguageClient('axonLspClient', 'Axon Language Server', serverOptions, clientOptions);
    // Start the client. This will also launch the server process.
    client.start();
}
function deactivate() {
    if (!client) {
        return undefined;
    }
    // Ensure we gracefully shut down the language server when VS Code closes
    return client.stop();
}
//# sourceMappingURL=extension.js.map