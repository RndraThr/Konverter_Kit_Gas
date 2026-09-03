import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, rmSync } from 'node:fs';
import { resolve } from 'node:path';

export default function globalTeardown() {
  const root = resolve(process.cwd(), '..');
	const pidPath = resolve(root, '.cache/e2e-server.pid');
	if (existsSync(pidPath)) {
		const pid = Number(readFileSync(pidPath, 'utf8'));
		if (Number.isInteger(pid)) {
			try { process.kill(pid); } catch { /* server already stopped */ }
		}
		rmSync(pidPath, { force: true });
	}
  const localEnv = Object.fromEntries(readFileSync(resolve(root, '.env'), 'utf8')
    .split(/\r?\n/)
    .filter((line) => line && !line.startsWith('#') && line.includes('='))
    .map((line) => { const index = line.indexOf('='); return [line.slice(0, index), line.slice(index + 1).replace(/^"|"$/g, '')]; }));
  const testDatabase = localEnv.DATABASE_URL?.replace('/konkit?', '/konkit_test?') ?? '';
	const fixtureDirectory = resolve(root, '.cache/e2e');
	const storageDirectory = resolve(root, '.cache/e2e-storage');
	const seedExecutable = resolve(root, '.cache/e2eseed.exe');
  execFileSync(seedExecutable, ['cleanup', '-fixture-dir', fixtureDirectory], {
    cwd: root,
		env: { ...process.env, ...localEnv, GOCACHE: resolve(root, '.cache/go-build'), APP_ENV: 'test', DATABASE_URL: testDatabase, STORAGE_PATH: storageDirectory },
    stdio: 'ignore',
  });
	rmSync(storageDirectory, { recursive: true, force: true });
}
