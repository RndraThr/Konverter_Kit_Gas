import { defineConfig, devices } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const root = resolve(process.cwd(), '..');
const localEnv = Object.fromEntries(readFileSync(resolve(root, '.env'), 'utf8')
  .split(/\r?\n/)
  .filter((line) => line && !line.startsWith('#') && line.includes('='))
  .map((line) => { const index = line.indexOf('='); return [line.slice(0, index), line.slice(index + 1).replace(/^"|"$/g, '')]; }));
const testDatabase = localEnv.DATABASE_URL?.replace('/konkit?', '/konkit_test?');

export default defineConfig({
  testDir: './e2e',
  globalTeardown: './e2e/global-teardown.ts',
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: { baseURL: 'http://127.0.0.1:8082', trace: 'retain-on-failure', screenshot: 'only-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1366, height: 768 } } },
    { name: 'mobile', use: { ...devices['Pixel 5'], viewport: { width: 390, height: 844 } } },
  ],
  webServer: {
    command: 'go run ./cmd/migrate up && go run ./cmd/e2eseed && go run ./cmd/server',
    cwd: root,
    env: { ...process.env, ...localEnv, GOCACHE: resolve(root, '.cache/go-build'), APP_ENV: 'test', APP_ADDR: ':8082', APP_BASE_URL: 'http://127.0.0.1:8082', DATABASE_URL: testDatabase ?? '' },
    url: 'http://127.0.0.1:8082/api/v1/health',
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
