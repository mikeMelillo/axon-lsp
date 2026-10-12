const assert = require('node:assert/strict');
const { readFileSync } = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');
const vm = require('node:vm');

function harness() {
    let now = 100;
    let nextTimer = 0;
    const timers = new Map();
    const watchers = [];
    const notifications = [];
    const directories = new Set(['/work/root/output.xeto']);
    const uri = fsPath => ({ fsPath, scheme: 'file', toString: () => `file://${fsPath}` });
    const disposable = () => ({ dispose() {} });
    const context = { extensionMode: 2, subscriptions: [], asAbsolutePath: value => `/extension/${value}` };
    const vscode = {
        ExtensionMode: { Development: 2 },
        Uri: { file: uri },
        RelativePattern: class { constructor(base, pattern) { this.base = base; this.pattern = pattern; } },
        EventEmitter: class { event = () => {}; fire() {} dispose() {} },
        workspace: {
            workspaceFolders: [{ uri: uri('/work/root') }], textDocuments: [],
            getConfiguration: () => ({ get: (_, fallback) => fallback }),
            registerTextDocumentContentProvider: disposable,
            onDidChangeConfiguration: disposable, onDidChangeWorkspaceFolders: disposable,
            createFileSystemWatcher(pattern) {
                const listeners = {};
                const watcher = { pattern, listeners, disposed: false, dispose() { this.disposed = true; } };
                for (const event of ['Create', 'Change', 'Delete']) {
                    watcher[`onDid${event}`] = callback => {
                        listeners[event] = callback;
                        return { dispose: () => delete listeners[event] };
                    };
                }
                watchers.push(watcher);
                return watcher;
            }
        },
        window: { createOutputChannel: () => ({ appendLine() {} }) },
        commands: { registerCommand: disposable }, env: {}
    };
    const exports = {};
    vm.runInNewContext(readFileSync(path.join(__dirname, 'out/extension.js'), 'utf8'), {
        exports, process: { env: {}, arch: 'x64', platform: 'linux' }, console,
        Date: { now: () => now },
        setTimeout(callback, delay) { const id = ++nextTimer; timers.set(id, { callback, at: now + delay }); return id; },
        clearTimeout: id => timers.delete(id),
        require(name) {
            if (name === 'vscode') return vscode;
            if (name === 'path') return path;
            if (name === 'fs') return { existsSync: () => true, statSync: value => ({ isDirectory: () => directories.has(value) }) };
            if (name === 'vscode-languageclient/node') return {
                TransportKind: { stdio: 0 },
                LanguageClient: class {
                    onNotification() {} start() { return Promise.resolve(); }
                    sendNotification(method, params) { notifications.push({ method, params }); return Promise.resolve(); }
                }
            };
            throw new Error(`unexpected module ${name}`);
        }
    });
    exports.activate(context);
    return {
        watchers, notifications, timers,
        event: (kind, value) => watchers[1].listeners[kind](uri(value)),
        dispose: () => context.subscriptions.forEach(value => value.dispose()),
        async advance(duration) {
            const end = now + duration;
            while (true) {
                const next = [...timers].sort((a, b) => a[1].at - b[1].at)[0];
                if (!next || next[1].at > end) break;
                now = next[1].at;
                timers.delete(next[0]);
                next[1].callback();
                // Drain cross-realm awaits, including a queued batch's first send.
                for (let i = 0; i < 30; i++) await Promise.resolve();
            }
            now = end;
        }
    };
}

test('fresh events cannot restart detached retries or retain old paths', async () => {
    const h = harness();
    h.event('Change', '/work/root/first.xetolib');
    await h.advance(250);
    h.event('Change', '/work/root/second.xetolib');
    await h.advance(5000);
    const reconciles = h.notifications.filter(value => value.method === 'axonLsp/reconcileSources');
    assert.deepEqual(reconciles.map(value => Array.from(value.params.uris)), [
        ...Array(3).fill(['file:///work/root/first.xetolib']),
        ...Array(3).fill(['file:///work/root/second.xetolib'])
    ]);
    assert.equal(h.timers.size, 0);
});

test('regular creates are ignored, unknown deletes have no retries, directories do', async () => {
    const h = harness();
    h.event('Create', '/work/root/.git/index.lock');
    h.event('Create', '/work/root/output.json');
    await h.advance(500);
    assert.equal(h.notifications.length, 0);
    h.event('Delete', '/work/root/output.log');
    await h.advance(3000);
    assert.equal(h.notifications.length, 1);
    h.event('Create', '/work/root/output.xeto');
    await h.advance(3000);
    assert.equal(h.notifications.length, 4);
    assert.ok(h.notifications.every(value => value.method === 'axonLsp/reconcileSources'));
});

test('continuous events cannot defer the first flush beyond the debounce cap', async () => {
    const h = harness();
    for (let i = 0; i < 10; i++) {
        h.event('Change', '/work/root/library.xetolib');
        await h.advance(100);
    }
    assert.equal(h.notifications.length, 1);
    await h.advance(3000);
    assert.equal(h.notifications.length, 3);
    assert.equal(h.timers.size, 0);
});

test('watch patterns are bounded and parent events and timer disposal work', async () => {
    const h = harness();
    const pattern = h.watchers[1].pattern;
    assert.equal(pattern.base.fsPath, '/');
    assert.ok(pattern.pattern.includes('work,'));
    assert.ok(pattern.pattern.includes('work/root/*/*/*/*/*'));
    assert.ok(!pattern.pattern.includes('**'));
    h.event('Delete', '/work');
    await h.advance(250);
    assert.deepEqual(Array.from(h.notifications[0].params.uris), ['file:///work']);
    h.event('Change', '/work/root/first.xetolib');
    await h.advance(250);
    h.event('Change', '/work/root/second.xetolib');
    h.dispose();
    assert.equal(h.timers.size, 0);
    assert.ok(h.watchers.every(watcher => watcher.disposed));
    const count = h.notifications.length;
    await h.advance(5000);
    assert.equal(h.notifications.length, count);
});
