import 'fake-indexeddb/auto';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { uploadRequest } from './upload';
import type { UploadTarget } from './uploadQueue';

vi.mock('./upload', () => ({ uploadRequest: vi.fn() }));

const slot: UploadTarget = { kind: 'distribution-slot', slotId: 'slot-1' };
const activity: UploadTarget = { kind: 'activity', programId: 'program-1', regencyId: 'regency-1', activityType: 'rakor' };

async function freshQueue() {
  const module = await import('./uploadQueue');
  module.uploadQueue.start();
  return module.uploadQueue;
}

function photo(name: string) {
  return new File([`bytes-${name}`], name, { type: 'image/jpeg' });
}

async function clearDatabase() {
  await new Promise<void>((resolve) => {
    const request = indexedDB.deleteDatabase('konkit-upload-queue');
    request.onsuccess = () => resolve();
    request.onerror = () => resolve();
    request.onblocked = () => resolve();
  });
}

beforeEach(async () => {
  vi.mocked(uploadRequest).mockReset();
  vi.resetModules();
  await clearDatabase();
});

afterEach(async () => {
  const { uploadQueue } = await import('./uploadQueue');
  uploadQueue.resetForTests();
});

test('uploads queued files one at a time, in order, with their form fields before the file', async () => {
  const resolvers: (() => void)[] = [];
  vi.mocked(uploadRequest).mockImplementation((_path, body) => new Promise((resolve) => { resolvers.push(() => resolve({ data: { id: (body.get('file') as File).name } })); }));
  const queue = await freshQueue();
  const completed: string[] = [];
  queue.onComplete(({ response }) => completed.push((response as { data: { id: string } }).data.id));

  queue.enqueue({ target: slot, label: 'Slot', fields: [['source', 'camera'], ['file_size', '9']], file: photo('first.jpg') });
  queue.enqueue({ target: activity, label: 'Rakor', fields: [['program_id', 'program-1']], file: photo('second.jpg') });

  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(1));
  const [path, body] = vi.mocked(uploadRequest).mock.calls[0];
  expect(path).toBe('/api/v1/distribution/slots/slot-1/media');
  expect(Array.from(body.keys())).toEqual(['source', 'file_size', 'file']);
  expect(queue.getSnapshot().map((job) => job.status)).toEqual(['uploading', 'queued']);

  resolvers[0]();
  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(2));
  expect(vi.mocked(uploadRequest).mock.calls[1][0]).toBe('/api/v1/activities/media');
  resolvers[1]();
  await vi.waitFor(() => expect(queue.getSnapshot()).toHaveLength(0));
  expect(completed).toEqual(['first.jpg', 'second.jpg']);
});

test('resumes an interrupted upload after the page is reloaded', async () => {
  vi.mocked(uploadRequest).mockImplementation(() => new Promise(() => undefined));
  const firstTab = await freshQueue();
  firstTab.enqueue({ target: slot, label: 'Slot', fields: [['source', 'gallery']], file: photo('bast.jpg') });
  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(1));
  // Wait until the job has been written to IndexedDB, as it would be before a real reload.
  await vi.waitFor(async () => {
    const stored = await new Promise<number>((resolve) => {
      const open = indexedDB.open('konkit-upload-queue', 1);
      open.onsuccess = () => { const count = open.result.transaction('jobs').objectStore('jobs').count(); count.onsuccess = () => { resolve(count.result); open.result.close(); }; };
    });
    expect(stored).toBe(1);
  });

  // "Reload": the old page is gone and a new module instance starts from storage.
  firstTab.resetForTests();
  vi.resetModules();
  vi.mocked(uploadRequest).mockReset();
  vi.mocked(uploadRequest).mockResolvedValue({ data: { id: 'media-1' } });
  const reloaded = await freshQueue();

  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(1));
  const [path, body] = vi.mocked(uploadRequest).mock.calls[0];
  expect(path).toBe('/api/v1/distribution/slots/slot-1/media');
  expect(body.get('source')).toBe('gallery');
  expect((body.get('file') as File).name).toBe('bast.jpg');
  await vi.waitFor(() => expect(reloaded.getSnapshot()).toHaveLength(0));
});

test('cancelling the active upload aborts it and moves on to the next file', async () => {
  const signals: AbortSignal[] = [];
  vi.mocked(uploadRequest).mockImplementation((_path, _body, options) => {
    signals.push(options!.signal!);
    return new Promise((_resolve, reject) => options?.signal?.addEventListener('abort', () => reject(new DOMException('cancelled', 'AbortError'))));
  });
  const queue = await freshQueue();
  const first = queue.enqueue({ target: slot, label: 'Slot', fields: [], file: photo('a.jpg') });
  queue.enqueue({ target: slot, label: 'Slot', fields: [], file: photo('b.jpg') });
  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(1));

  queue.cancel(first.id);

  expect(signals[0].aborted).toBe(true);
  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(2));
  expect(queue.getSnapshot().map((job) => job.file.name)).toEqual(['b.jpg']);
});

test('a rejected upload waits for a manual retry while a dropped connection retries by itself', async () => {
  const { ApiError } = await import('./api');
  vi.mocked(uploadRequest)
    .mockRejectedValueOnce(new ApiError(409, 'slot_full', 'Slot sudah penuh'))
    .mockRejectedValueOnce(new Error('Koneksi terputus saat mengunggah berkas'))
    .mockResolvedValue({ data: {} });
  const queue = await freshQueue();
  const rejected = queue.enqueue({ target: slot, label: 'Slot', fields: [], file: photo('a.jpg') });
  queue.enqueue({ target: slot, label: 'Slot', fields: [], file: photo('b.jpg') });

  await vi.waitFor(() => expect(queue.getSnapshot().every((job) => job.status === 'error')).toBe(true));
  expect(queue.getSnapshot().map((job) => [job.errorMessage, job.autoRetry])).toEqual([['Slot sudah penuh', false], ['Koneksi terputus saat mengunggah berkas', true]]);

  // Coming back online retries only the connection failure.
  window.dispatchEvent(new Event('online'));
  await vi.waitFor(() => expect(queue.getSnapshot().map((job) => job.file.name)).toEqual(['a.jpg']));

  queue.retry(rejected.id);
  await vi.waitFor(() => expect(queue.getSnapshot()).toHaveLength(0));
  expect(uploadRequest).toHaveBeenCalledTimes(4);
});

test('an expired session keeps the file queued instead of failing it', async () => {
  const { ApiError } = await import('./api');
  vi.mocked(uploadRequest).mockRejectedValue(new ApiError(401, 'unauthorized', 'Sesi login telah berakhir'));
  const queue = await freshQueue();
  queue.enqueue({ target: slot, label: 'Slot', fields: [], file: photo('a.jpg') });

  await vi.waitFor(() => expect(uploadRequest).toHaveBeenCalledTimes(1));
  await vi.waitFor(() => expect(queue.getSnapshot()[0].status).toBe('queued'));
});
