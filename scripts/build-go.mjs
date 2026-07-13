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

const targets = [
  ['linux', 'amd64', 'linux-x64'],
  ['linux', 'arm64', 'linux-arm64'],
  ['darwin', 'amd64', 'darwin-x64'],
  ['darwin', 'arm64', 'darwin-arm64'],
  ['windows', 'amd64', 'win32-x64'],
  ['windows', 'arm64', 'win32-arm64']
];

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
