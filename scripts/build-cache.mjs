import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '..');

const result = spawnSync('go', ['run', './cmd/cache-builder', '--root', '..', ...process.argv.slice(2)], {
  cwd: path.join(repoRoot, 'go-server'),
  stdio: 'inherit',
  env: process.env
});

if (result.error) {
  console.error(result.error.message);
}
if (result.status !== 0) {
  process.exit(result.status ?? 1);
}
