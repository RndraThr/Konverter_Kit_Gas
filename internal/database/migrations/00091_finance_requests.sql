-- +goose Up
-- Pengajuan keuangan: leader tim mengajukan cashless (bayar langsung ke
-- rekening penerima) atau kasbon (uang muka yang wajib dilaporkan). Finance
-- mereview dan membayar pengajuan tim yang ditugaskan kepadanya.
INSERT INTO permissions (code, name, description) VALUES
    ('finance.request', 'Ajukan Keuangan', 'Membuat pengajuan cashless dan kasbon untuk tim yang dipimpin'),
    ('finance.review', 'Review Keuangan', 'Mereview, membayar, dan melaporkan pengajuan tim yang ditugaskan'),
    ('finance.monitor', 'Monitoring Keuangan', 'Melihat dan mengekspor seluruh pengajuan keuangan'),
    ('finance.manage', 'Kelola Tim Keuangan', 'Mengatur tim, leader, finance, dan kabupaten pengajuan')
ON CONFLICT (code) DO NOTHING;

INSERT INTO roles (code, name, description, is_system)
VALUES ('finance', 'Finance', 'Mereview dan membayar pengajuan tim yang ditugaskan', false)
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT roles.id, permissions.id FROM roles CROSS JOIN permissions
WHERE (roles.code = 'finance' AND permissions.code IN ('dashboard.view', 'finance.review'))
   OR (roles.code = 'admin_leader' AND permissions.code = 'finance.request')
   OR (roles.code = 'admin' AND permissions.code IN ('finance.monitor', 'finance.manage'))
   OR (roles.code = 'super_admin' AND permissions.code IN ('finance.request', 'finance.review', 'finance.monitor', 'finance.manage'))
ON CONFLICT DO NOTHING;

CREATE TABLE finance_teams (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX finance_teams_name_key ON finance_teams (lower(name));

-- member_role: leader mengajukan, finance menangani. Satu orang boleh ada di
-- beberapa tim, dan satu finance menangani banyak tim.
CREATE TABLE finance_team_members (
    team_id uuid NOT NULL REFERENCES finance_teams(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    member_role text NOT NULL CHECK (member_role IN ('leader', 'finance')),
    PRIMARY KEY (team_id, user_id, member_role)
);
CREATE INDEX finance_team_members_user_idx ON finance_team_members (user_id, member_role);

CREATE TABLE finance_team_regencies (
    team_id uuid NOT NULL REFERENCES finance_teams(id) ON DELETE CASCADE,
    regency_id uuid NOT NULL REFERENCES regencies(id) ON DELETE CASCADE,
    PRIMARY KEY (team_id, regency_id)
);

CREATE TABLE finance_request_counters (
    period text PRIMARY KEY,
    last_value integer NOT NULL
);

CREATE TABLE finance_requests (
    id uuid PRIMARY KEY,
    number text UNIQUE,
    request_type text NOT NULL CHECK (request_type IN ('cashless', 'kasbon')),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN (
        'draft', 'submitted', 'revision', 'rejected', 'cancelled', 'approved',
        'paid', 'settlement_submitted', 'settlement_revision', 'done')),
    team_id uuid REFERENCES finance_teams(id) ON DELETE RESTRICT,
    requester_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    regency_id uuid REFERENCES regencies(id) ON DELETE SET NULL,
    needed_date date,
    category text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    total_amount bigint NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    payee_name text NOT NULL DEFAULT '',
    bank_name text NOT NULL DEFAULT '',
    account_number text NOT NULL DEFAULT '',
    account_name text NOT NULL DEFAULT '',
    invoice_number text NOT NULL DEFAULT '',
    settlement_due date,
    settlement_amount bigint CHECK (settlement_amount >= 0),
    settlement_note text NOT NULL DEFAULT '',
    review_note text NOT NULL DEFAULT '',
    submitted_at timestamptz,
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    paid_by uuid REFERENCES users(id) ON DELETE SET NULL,
    paid_at timestamptz,
    done_at timestamptz,
    reminded_on date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX finance_requests_requester_idx ON finance_requests (requester_id, updated_at DESC);
CREATE INDEX finance_requests_team_status_idx ON finance_requests (team_id, status, submitted_at DESC);

-- phase: request = rincian pengajuan, settlement = realisasi kasbon.
CREATE TABLE finance_request_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL REFERENCES finance_requests(id) ON DELETE CASCADE,
    phase text NOT NULL CHECK (phase IN ('request', 'settlement')),
    position integer NOT NULL,
    description text NOT NULL DEFAULT '',
    quantity integer NOT NULL DEFAULT 1 CHECK (quantity >= 0),
    unit_price bigint NOT NULL DEFAULT 0 CHECK (unit_price >= 0),
    amount bigint NOT NULL DEFAULT 0 CHECK (amount >= 0),
    UNIQUE (request_id, phase, position)
);

-- kind: evidence = nota/penawaran, payment = bukti bayar, settlement = nota
-- realisasi kasbon. Berkasnya di penyimpanan media, bukan di database.
CREATE TABLE finance_request_attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL REFERENCES finance_requests(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('evidence', 'payment', 'settlement')),
    storage_key text NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX finance_request_attachments_request_idx ON finance_request_attachments (request_id, kind);

CREATE TABLE finance_request_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id uuid NOT NULL REFERENCES finance_requests(id) ON DELETE CASCADE,
    action text NOT NULL,
    from_status text NOT NULL,
    to_status text NOT NULL,
    note text NOT NULL DEFAULT '',
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX finance_request_events_request_idx ON finance_request_events (request_id, created_at);
CREATE INDEX finance_request_events_created_idx ON finance_request_events (created_at);

-- +goose Down
DROP TABLE finance_request_events;
DROP TABLE finance_request_attachments;
DROP TABLE finance_request_items;
DROP TABLE finance_requests;
DROP TABLE finance_request_counters;
DROP TABLE finance_team_regencies;
DROP TABLE finance_team_members;
DROP TABLE finance_teams;
DELETE FROM roles WHERE code = 'finance';
DELETE FROM permissions WHERE code IN ('finance.request', 'finance.review', 'finance.monitor', 'finance.manage');
