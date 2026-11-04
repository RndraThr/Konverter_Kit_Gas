import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

afterEach(cleanup);

// The upload queue is an app-wide singleton; give every test an empty one.
// Imported lazily so test files that mock lib/upload get the mocked module.
afterEach(async () => {
  const { uploadQueue } = await import('../lib/uploadQueue');
  uploadQueue.resetForTests();
});
