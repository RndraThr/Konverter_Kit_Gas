import { useEffect, useRef, useSyncExternalStore } from 'react';
import { ApiError } from './api';
import { uploadRequest } from './upload';

// Antrian unggah dokumentasi (foto/video) tingkat aplikasi. Berkas yang dipilih
// langsung disimpan di IndexedDB lalu diunggah satu per satu, jadi unggahan
// tetap berjalan saat pengguna pindah menu dan dilanjutkan setelah halaman
// dimuat ulang. Bila beberapa tab terbuka, hanya satu tab (pemegang Web Lock)
// yang mengunggah; tab lain melihat antrian yang sama lewat BroadcastChannel.

export type UploadTarget =
  | { kind: 'distribution-slot'; slotId: string }
  | { kind: 'activity'; programId: string; regencyId: string; activityType: string };

export type UploadJobStatus = 'queued' | 'uploading' | 'error';

export type UploadJob = {
  id: string;
  target: UploadTarget;
  /** Nama tujuan yang dibaca manusia, mis. "Pendistribusian · BAST bertanda tangan". */
  label: string;
  /** Kolom form sesuai urutan; berkas selalu dikirim paling akhir. */
  fields: [string, string][];
  file: File;
  createdAt: number;
  status: UploadJobStatus;
  progress: number;
  errorMessage?: string;
  /** Kegagalan jaringan atau server sibuk: dicoba lagi otomatis. */
  autoRetry?: boolean;
};

export type NewUploadJob = Pick<UploadJob, 'target' | 'label' | 'fields' | 'file'>;
export type UploadCompletion = { job: UploadJob; response: unknown };

// Bentuk yang disimpan: nama dan tipe berkas ikut disimpan terpisah agar File
// dapat disusun ulang utuh saat antrian dimuat kembali.
type StoredJob = Omit<UploadJob, 'file'> & { file: Blob; fileName: string; fileType: string; fileLastModified: number };

function toStored(job: UploadJob): StoredJob {
  return { ...job, progress: 0, file: job.file, fileName: job.file.name, fileType: job.file.type, fileLastModified: job.file.lastModified };
}

function fromStored(stored: StoredJob): UploadJob {
  const { fileName, fileType, fileLastModified, file, ...job } = stored;
  const restored = file instanceof File && file.name === fileName ? file : new File([file], fileName, { type: fileType, lastModified: fileLastModified });
  return { ...job, file: restored };
}

type ChannelMessage =
  | { type: 'changed' }
  | { type: 'progress'; id: string; progress: number }
  | { type: 'completed'; job: UploadJob; response: unknown };

const DB_NAME = 'konkit-upload-queue';
const STORE = 'jobs';
const LOCK_NAME = 'konkit-upload-queue';
const AUTO_RETRY_MS = 15_000;
const RETRYABLE_STATUSES = new Set([0, 408, 429, 502, 503, 504]);

export function uploadPath(target: UploadTarget) {
  return target.kind === 'distribution-slot'
    ? `/api/v1/distribution/slots/${encodeURIComponent(target.slotId)}/media`
    : '/api/v1/activities/media';
}

export function sameTarget(a: UploadTarget, b: UploadTarget) {
  if (a.kind === 'distribution-slot' && b.kind === 'distribution-slot') return a.slotId === b.slotId;
  if (a.kind === 'activity' && b.kind === 'activity') return a.programId === b.programId && a.regencyId === b.regencyId && a.activityType === b.activityType;
  return false;
}

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  return new Promise((resolve) => {
    try {
      const request = indexedDB.open(DB_NAME, 1);
      request.onupgradeneeded = () => { request.result.createObjectStore(STORE, { keyPath: 'id' }); };
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => resolve(null);
      request.onblocked = () => resolve(null);
    } catch {
      resolve(null);
    }
  });
}

function runStore<T>(db: IDBDatabase, mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T> | void): Promise<T | undefined> {
  return new Promise((resolve) => {
    try {
      const transaction = db.transaction(STORE, mode);
      const request = action(transaction.objectStore(STORE));
      transaction.oncomplete = () => resolve(request ? request.result : undefined);
      transaction.onerror = () => resolve(undefined);
      transaction.onabort = () => resolve(undefined);
    } catch {
      resolve(undefined);
    }
  });
}

function isRetryable(error: unknown) {
  if (error instanceof ApiError) return RETRYABLE_STATUSES.has(error.status);
  return true; // Koneksi terputus atau galat tak dikenal dari XHR.
}

class UploadQueue {
  private jobs: UploadJob[] = [];
  private listeners = new Set<() => void>();
  private completionListeners = new Set<(completion: UploadCompletion) => void>();
  private active: { id: string; controller: AbortController } | null = null;
  private database: Promise<IDBDatabase | null> | null = null;
  private channel: BroadcastChannel | null = null;
  private started = false;
  private leader = false;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private generation = 0;
  // Perubahan yang belum selesai ditulis ke IndexedDB, agar muat ulang dari
  // tab lain tidak memunculkan kembali atau menghapus pekerjaan sebelum waktunya.
  private unsaved = new Set<string>();
  private removing = new Set<string>();

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    this.start();
    return () => { this.listeners.delete(listener); };
  };

  getSnapshot = () => this.jobs;

  onComplete(listener: (completion: UploadCompletion) => void) {
    this.completionListeners.add(listener);
    return () => { this.completionListeners.delete(listener); };
  }

  /** Memuat antrian tersimpan dan mulai mengunggah. Aman dipanggil berkali-kali. */
  start() {
    if (this.started) return;
    this.started = true;
    const generation = this.generation;
    this.database = openDatabase();
    void this.database.then(async (db) => {
      if (generation !== this.generation) return;
      if (db && typeof BroadcastChannel !== 'undefined') {
        this.channel = new BroadcastChannel(DB_NAME);
        this.channel.onmessage = (event: MessageEvent<ChannelMessage>) => this.handleMessage(event.data);
      }
      await this.reload();
      if (typeof window !== 'undefined') window.addEventListener('online', this.retryWaiting);
      this.acquireLeadership();
    });
  }

  enqueue(input: NewUploadJob): UploadJob {
    this.start();
    // createdAt selalu naik agar urutan antrian tetap sama setelah dimuat dari IndexedDB.
    const createdAt = Math.max(Date.now(), ...this.jobs.map((item) => item.createdAt + 1));
    const job: UploadJob = { ...input, id: crypto.randomUUID(), createdAt, status: 'queued', progress: 0 };
    this.unsaved.add(job.id);
    this.setJobs([...this.jobs, job]);
    void this.persist(job).then(() => { this.unsaved.delete(job.id); this.broadcast({ type: 'changed' }); this.pump(); });
    this.pump();
    return job;
  }

  retry(id: string) {
    const job = this.jobs.find((item) => item.id === id);
    if (!job || job.status !== 'error') return;
    this.update(id, { status: 'queued', progress: 0, errorMessage: undefined, autoRetry: false });
    this.pump();
  }

  cancel(id: string) {
    if (this.active?.id === id) { this.active.controller.abort(); this.active = null; }
    this.setJobs(this.jobs.filter((item) => item.id !== id));
    void this.remove(id).then(() => this.broadcast({ type: 'changed' }));
    this.pump();
  }

  resetForTests() {
    this.generation += 1;
    this.active?.controller.abort();
    this.active = null;
    if (this.retryTimer) clearTimeout(this.retryTimer);
    this.retryTimer = null;
    this.channel?.close();
    this.channel = null;
    if (typeof window !== 'undefined') window.removeEventListener('online', this.retryWaiting);
    void this.database?.then((db) => db?.close());
    this.jobs = [];
    this.unsaved.clear();
    this.removing.clear();
    this.completionListeners.clear();
    this.database = null;
    this.started = false;
    this.leader = false;
  }

  private setJobs(next: UploadJob[]) {
    this.jobs = next;
    this.listeners.forEach((listener) => listener());
  }

  private update(id: string, patch: Partial<UploadJob>, persist = true) {
    let changed: UploadJob | undefined;
    this.setJobs(this.jobs.map((item) => {
      if (item.id !== id) return item;
      changed = { ...item, ...patch };
      return changed;
    }));
    if (persist && changed) {
      const saved = changed;
      void this.persist(saved).then(() => this.broadcast({ type: 'changed' }));
    }
  }

  private async persist(job: UploadJob) {
    const db = await this.database;
    if (db) await runStore(db, 'readwrite', (store) => store.put(toStored(job)));
  }

  private async remove(id: string) {
    this.removing.add(id);
    const db = await this.database;
    if (db) await runStore(db, 'readwrite', (store) => store.delete(id));
    this.removing.delete(id);
  }

  // Menyamakan antrian di memori dengan IndexedDB. Unggahan yang sedang
  // berjalan di tab ini tetap memakai status dan progres di memori.
  private async reload() {
    const db = await this.database;
    if (!db) return;
    const stored = (await runStore<StoredJob[]>(db, 'readonly', (store) => store.getAll() as IDBRequest<StoredJob[]>)) ?? [];
    const current = new Map(this.jobs.map((item) => [item.id, item]));
    const merged = stored
      .filter((item) => !this.removing.has(item.id))
      .map((saved): UploadJob => {
        const local = current.get(saved.id);
        if (this.active?.id === saved.id && local) return local;
        // Berkas di memori dipakai bila ada; IndexedDB hanya menentukan status terbaru.
        const { file: _file, fileName: _name, fileType: _type, fileLastModified: _modified, ...state } = saved;
        const item = local ? { ...local, ...state, file: local.file } : fromStored(saved);
        // Status "uploading" yang tersimpan berasal dari tab yang ditutup atau dimuat ulang.
        if (item.status === 'uploading' && this.leader) return { ...item, status: 'queued', progress: 0 };
        return local && local.status === 'uploading' && !this.leader ? { ...item, progress: local.progress } : item;
      })
      .sort((a, b) => a.createdAt - b.createdAt);
    if (this.active && !merged.some((item) => item.id === this.active?.id)) {
      this.active.controller.abort();
      this.active = null;
    }
    const pendingLocal = this.jobs.filter((item) => this.unsaved.has(item.id) && !stored.some((saved) => saved.id === item.id));
    this.setJobs([...merged, ...pendingLocal].sort((a, b) => a.createdAt - b.createdAt));
  }

  private acquireLeadership() {
    const generation = this.generation;
    const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined;
    if (!locks || !this.channel) {
      this.becomeLeader();
      return;
    }
    void locks.request(LOCK_NAME, () => {
      if (generation !== this.generation) return undefined;
      this.becomeLeader();
      // Kunci dipegang selama tab ini terbuka.
      return new Promise<void>(() => undefined);
    }).catch(() => this.becomeLeader());
  }

  private becomeLeader() {
    this.leader = true;
    this.setJobs(this.jobs.map((item) => item.status === 'uploading' && this.active?.id !== item.id ? { ...item, status: 'queued', progress: 0 } : item));
    this.pump();
  }

  private handleMessage(message: ChannelMessage) {
    if (message.type === 'changed') void this.reload().then(() => this.pump());
    else if (message.type === 'progress') this.update(message.id, { status: 'uploading', progress: message.progress }, false);
    else if (message.type === 'completed') this.completionListeners.forEach((listener) => listener({ job: message.job, response: message.response }));
  }

  private broadcast(message: ChannelMessage) {
    try { this.channel?.postMessage(message); } catch { /* tab lain tidak wajib menerima */ }
  }

  private retryWaiting = () => {
    let requeued = false;
    this.jobs.forEach((item) => {
      if (item.status === 'error' && item.autoRetry) { requeued = true; this.update(item.id, { status: 'queued', progress: 0, errorMessage: undefined, autoRetry: false }); }
    });
    if (requeued) this.pump();
  };

  private scheduleAutoRetry() {
    if (this.retryTimer) return;
    this.retryTimer = setTimeout(() => { this.retryTimer = null; this.retryWaiting(); }, AUTO_RETRY_MS);
  }

  private pump() {
    if (!this.leader || this.active) return;
    const next = this.jobs.find((item) => item.status === 'queued');
    if (!next) return;
    const controller = new AbortController();
    const generation = this.generation;
    this.active = { id: next.id, controller };
    this.update(next.id, { status: 'uploading', progress: 0, errorMessage: undefined });
    const body = new FormData();
    next.fields.forEach(([key, value]) => body.set(key, value));
    body.set('file', next.file);
    let lastBroadcast = 0;
    uploadRequest<unknown>(uploadPath(next.target), body, {
      signal: controller.signal,
      onProgress: (progress) => {
        if (this.active?.id !== next.id) return;
        this.update(next.id, { progress }, false);
        const now = Date.now();
        if (now - lastBroadcast > 500 || progress === 100) { lastBroadcast = now; this.broadcast({ type: 'progress', id: next.id, progress }); }
      },
    }).then((response) => {
      if (generation !== this.generation || this.active?.id !== next.id) return;
      this.active = null;
      this.setJobs(this.jobs.filter((item) => item.id !== next.id));
      const completion = { job: next, response };
      this.completionListeners.forEach((listener) => listener(completion));
      void this.remove(next.id).then(() => { this.broadcast({ type: 'completed', ...completion }); this.broadcast({ type: 'changed' }); });
      this.pump();
    }, (error: unknown) => {
      if (generation !== this.generation) return;
      if (error instanceof DOMException && error.name === 'AbortError') return; // dibatalkan pengguna
      if (this.active?.id !== next.id) return;
      this.active = null;
      if (error instanceof ApiError && error.status === 401) {
        // Halaman dialihkan ke login; antrian dilanjutkan setelah masuk lagi.
        this.update(next.id, { status: 'queued', progress: 0 });
        return;
      }
      const autoRetry = isRetryable(error);
      const errorMessage = error instanceof Error ? error.message : 'Media belum dapat diunggah.';
      this.update(next.id, { status: 'error', progress: 0, errorMessage, autoRetry });
      if (autoRetry) this.scheduleAutoRetry();
      this.pump();
    });
  }
}

export const uploadQueue = new UploadQueue();

export function useUploadJobs(): UploadJob[] {
  return useSyncExternalStore(uploadQueue.subscribe, uploadQueue.getSnapshot, uploadQueue.getSnapshot);
}

/** Memanggil handler setiap kali sebuah unggahan selesai, termasuk dari tab lain. */
export function useUploadCompletions(handler: (completion: UploadCompletion) => void) {
  const handlerRef = useRef(handler);
  useEffect(() => { handlerRef.current = handler; });
  useEffect(() => uploadQueue.onComplete((completion) => handlerRef.current(completion)), []);
}
