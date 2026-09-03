import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

export default async function globalSetup() {
  const root = resolve(process.cwd(), '..');
  const cache = resolve(root, '.cache');
  const seedExecutable = resolve(cache, 'e2eseed.exe');
  const serverExecutable = resolve(cache, 'e2e-server.exe');
  const fixtureDirectory = process.env.E2E_FIXTURE_DIR ?? resolve(cache, 'e2e');
  mkdirSync(cache, { recursive: true });

  for (const args of [
    ['run', './cmd/migrate', 'up'],
    ['build', '-o', seedExecutable, './cmd/e2eseed'],
    ['build', '-o', serverExecutable, './cmd/server'],
  ]) {
    const result = spawnSync('go', args, { cwd: root, env: process.env, stdio: 'inherit' });
    if (result.status !== 0) throw new Error(`E2E preparation failed: go ${args.join(' ')}`);
  }
  const seed = spawnSync(seedExecutable, ['-fixture-dir', fixtureDirectory], { cwd: root, env: process.env, stdio: 'inherit' });
  if (seed.status !== 0) throw new Error('E2E seed failed');

  const server = spawn(serverExecutable, [], { cwd: root, env: process.env, stdio: 'ignore', windowsHide: true });
  if (!server.pid) throw new Error('E2E server did not start');
  writeFileSync(resolve(cache, 'e2e-server.pid'), String(server.pid));
  for (let attempt = 0; attempt < 60; attempt++) {
    try {
      const response = await fetch('http://127.0.0.1:8082/api/v1/health');
      if (response.ok) return;
    } catch { /* server is still starting */ }
    await new Promise((resolveWait) => setTimeout(resolveWait, 500));
  }
  process.kill(server.pid);
  throw new Error('E2E server health check timed out');
}
