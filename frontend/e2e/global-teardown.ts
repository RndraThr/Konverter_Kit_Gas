import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export default function globalTeardown() {
  const root = resolve(process.cwd(), '..');
  const localEnv = Object.fromEntries(readFileSync(resolve(root, '.env'), 'utf8')
    .split(/\r?\n/)
    .filter((line) => line && !line.startsWith('#') && line.includes('='))
    .map((line) => { const index = line.indexOf('='); return [line.slice(0, index), line.slice(index + 1).replace(/^"|"$/g, '')]; }));
  const testDatabase = localEnv.DATABASE_URL?.replace('/konkit?', '/konkit_test?') ?? '';
  execFileSync('go', ['run', './cmd/e2eseed', 'cleanup'], {
    cwd: root,
    env: { ...process.env, ...localEnv, GOCACHE: resolve(root, '.cache/go-build'), APP_ENV: 'test', DATABASE_URL: testDatabase },
    stdio: 'ignore',
  });
}
