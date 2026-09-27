import { spawnSync } from 'node:child_process';
import { chmodSync, existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');
const outputDir = path.join(repoRoot, 'bin');

if (!existsSync(outputDir)) {
  mkdirSync(outputDir, { recursive: true });
}

const allTargets = [
  ['linux', 'amd64', 'linux-x64'],
  ['linux', 'arm64', 'linux-arm64'],
  ['darwin', 'amd64', 'darwin-x64'],
  ['darwin', 'arm64', 'darwin-arm64'],
  ['windows', 'amd64', 'win32-x64'],
  ['windows', 'arm64', 'win32-arm64']
];

// --host builds only the binary this machine actually runs, which is the only
// one the extension loads (see resolveServerBinary in client/src/extension.ts).
// The full cross-compile stays the default because the published VSIX has to
// carry a binary for every platform.
let targets = allTargets;
if (process.argv.includes('--host')) {
  // Node reports arch as x64/arm64, Go as amd64/arm64.
  const goArch = { x64: 'amd64', arm64: 'arm64' }[process.arch];
  const host = allTargets.find(([goos, goarch]) => goos === process.platform && goarch === goArch);
  if (!host) {
    console.error(`Unsupported host platform: ${process.platform}/${process.arch}`);
    process.exit(1);
  }
  targets = [host];
  console.log(`Host-only build: ${host[2]}`);
}

for (const [goos, goarch, label] of targets) {
  const fileName = goos === 'windows' ? `axon-lsp-${label}.exe` : `axon-lsp-${label}`;
  const outputPath = path.join(outputDir, fileName);
  const result = spawnSync('go', ['build', '-o', outputPath, './cmd/axon-lsp'], {
    cwd: path.join(repoRoot, 'go-server'),
    stdio: 'inherit',
    env: {
      ...process.env,
      CGO_ENABLED: '0',
      GOOS: goos,
      GOARCH: goarch
    }
  });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
  if (goos !== 'windows') {
    chmodSync(outputPath, 0o755);
  }
}
