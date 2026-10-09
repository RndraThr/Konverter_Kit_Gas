export type AuditEntry = {
  id: string;
  actor_user_id?: string;
  actor_name?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  metadata: Record<string, unknown>;
  ip_address?: string;
  user_agent?: string;
  created_at: string;
};

const actionLabels: Record<string, string> = {
  'settings.updated': 'Memperbarui pengaturan sistem',
  'user.created': 'Membuat pengguna',
  'user.updated': 'Memperbarui pengguna',
  'auth.login': 'Masuk ke sistem',
  'distribution.completed': 'Menyelesaikan pendistribusian',
  'media.uploaded': 'Mengunggah dokumentasi',
};

const resourceLabels: Record<string, string> = {
  system_settings: 'Pengaturan sistem',
  users: 'Pengguna',
  distribution_slots: 'Slot pendistribusian',
  media_files: 'Dokumentasi media',
};

export function presentAuditAction(action: string) {
  return { label: actionLabels[action] ?? 'Aktivitas sistem', code: action };
}

export function presentActor(actorName?: string) {
  return actorName?.trim() || 'Sistem';
}

export function presentResource(resourceType: string) {
  return resourceLabels[resourceType] ?? resourceType.replaceAll('_', ' ');
}

function sortedValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortedValue);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => [key, sortedValue(item)]));
  }
  return value;
}

export function metadataRows(metadata: Record<string, unknown>) {
  return Object.entries(metadata).sort(([left], [right]) => left.localeCompare(right));
}

export function stableAuditJSON(metadata: Record<string, unknown>) {
  return JSON.stringify(sortedValue(metadata), null, 2);
}

export function readableMetadataValue(value: unknown) {
  if (value === null) return 'Kosong';
  if (typeof value === 'boolean') return value ? 'Ya' : 'Tidak';
  if (typeof value === 'object') return JSON.stringify(sortedValue(value));
  return String(value);
}
