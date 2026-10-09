import { expect, test } from 'vitest';
import { metadataRows, presentActor, presentAuditAction, presentResource, stableAuditJSON } from './auditPresentation';

test('presents known and unknown audit actions without losing their codes', () => {
  expect(presentAuditAction('settings.updated')).toEqual({ label: 'Memperbarui pengaturan sistem', code: 'settings.updated' });
  expect(presentAuditAction('custom.event')).toEqual({ label: 'Aktivitas sistem', code: 'custom.event' });
});

test('uses readable fallbacks for actors and resources', () => {
  expect(presentActor('')).toBe('Sistem');
  expect(presentResource('system_settings')).toBe('Pengaturan sistem');
  expect(presentResource('custom_resource')).toBe('custom resource');
});

test('formats metadata as rows and stable JSON', () => {
  const metadata = { zebra: 2, alpha: { enabled: true } };
  expect(metadataRows(metadata).map(([key]) => key)).toEqual(['alpha', 'zebra']);
  expect(stableAuditJSON(metadata)).toBe('{\n  "alpha": {\n    "enabled": true\n  },\n  "zebra": 2\n}');
});
