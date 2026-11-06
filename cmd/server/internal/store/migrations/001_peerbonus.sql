CREATE TABLE program (
    id smallint PRIMARY KEY CHECK (id = 1),
    time_zone text NOT NULL,
    weekly_allowance integer NOT NULL CHECK (weekly_allowance BETWEEN 0 AND 10000),
    max_points_per_post integer NOT NULL CHECK (max_points_per_post BETWEEN 1 AND weekly_allowance),
    point_unit_value numeric NOT NULL CHECK (point_unit_value >= 0),
    currency text NOT NULL,
    values jsonb NOT NULL DEFAULT '[]',
    updated_at timestamptz NOT NULL,
    updated_by text NOT NULL DEFAULT ''
);
INSERT INTO program (id, time_zone, weekly_allowance, max_points_per_post, point_unit_value, currency, updated_at)
VALUES (1, 'Asia/Tokyo', 400, 120, 1, 'JPY', CURRENT_TIMESTAMP);

CREATE TABLE members (
    id text PRIMARY KEY,
    person_id text NOT NULL UNIQUE,
    subject text UNIQUE,
    display_name text NOT NULL,
    email text NOT NULL,
    last_seen_at timestamptz
);
CREATE TABLE periods (
    key text PRIMARY KEY,
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL CHECK (ends_at > starts_at),
    allowance integer NOT NULL CHECK (allowance >= 0),
    max_points_per_post integer NOT NULL CHECK (max_points_per_post BETWEEN 1 AND allowance)
);
CREATE TABLE posts (
    id text PRIMARY KEY,
    period_key text NOT NULL REFERENCES periods(key),
    month_key text NOT NULL,
    sender_id text NOT NULL REFERENCES members(id),
    recipient_id text NOT NULL REFERENCES members(id),
    points integer NOT NULL CHECK (points > 0),
    message text NOT NULL CHECK (char_length(message) BETWEEN 1 AND 500),
    value_keys text[] NOT NULL DEFAULT '{}',
    clap_count integer NOT NULL DEFAULT 0 CHECK (clap_count >= 0),
    created_at timestamptz NOT NULL,
    deleted_at timestamptz,
    deleted_by text REFERENCES members(id),
    reversed boolean NOT NULL DEFAULT false,
    CHECK (sender_id <> recipient_id),
    CHECK (cardinality(value_keys) <= 3)
);
CREATE INDEX posts_timeline ON posts (deleted_at, created_at DESC, id DESC);
CREATE INDEX posts_sender ON posts (sender_id, period_key, month_key);
CREATE INDEX posts_recipient ON posts (recipient_id, month_key);
CREATE TABLE claps (
    id text PRIMARY KEY,
    post_id text NOT NULL REFERENCES posts(id),
    member_id text NOT NULL REFERENCES members(id),
    period_key text NOT NULL REFERENCES periods(key),
    month_key text NOT NULL,
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX claps_post ON claps (post_id, created_at DESC, id DESC);
CREATE INDEX claps_member ON claps (member_id, period_key, month_key);
CREATE TABLE ledger (
    id bigserial PRIMARY KEY,
    member_id text NOT NULL REFERENCES members(id),
    period_key text NOT NULL REFERENCES periods(key),
    month_key text NOT NULL,
    account text NOT NULL CHECK (account IN ('give', 'receive')),
    delta integer NOT NULL CHECK (delta <> 0),
    kind text NOT NULL CHECK (kind IN ('send', 'receive', 'clap_give', 'clap_sender', 'clap_recipient', 'reversal')),
    post_id text NOT NULL REFERENCES posts(id),
    clap_id text REFERENCES claps(id),
    reverses_id bigint UNIQUE REFERENCES ledger(id),
    created_at timestamptz NOT NULL,
    CHECK ((kind = 'reversal') = (reverses_id IS NOT NULL)),
    CHECK (kind = 'reversal' OR (account = 'give' AND delta < 0) OR (account = 'receive' AND delta > 0))
);
CREATE INDEX ledger_member_period ON ledger (member_id, account, period_key);
CREATE INDEX ledger_member_month ON ledger (member_id, account, month_key);
CREATE INDEX ledger_month ON ledger (month_key);
CREATE INDEX ledger_post ON ledger (post_id);
CREATE TABLE statements (
    month_key text PRIMARY KEY,
    closed_at timestamptz NOT NULL,
    closed_by text NOT NULL REFERENCES members(id),
    point_unit_value numeric NOT NULL,
    currency text NOT NULL,
    total_points bigint NOT NULL CHECK (total_points >= 0),
    total_amount numeric NOT NULL CHECK (total_amount >= 0),
    member_count integer NOT NULL,
    directory_refreshed boolean NOT NULL
);
CREATE TABLE statement_lines (
    month_key text NOT NULL REFERENCES statements(month_key),
    member_id text NOT NULL REFERENCES members(id),
    person_id text NOT NULL,
    subject text,
    email text NOT NULL,
    display_name text NOT NULL,
    points bigint NOT NULL CHECK (points > 0),
    amount numeric NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (month_key, member_id)
);
-- The expiring index owns idempotency for both actions; claps remain immutable
-- when their request key expires, and no replayed response snapshot can go stale.
CREATE TABLE idempotency_keys (
    member_id text NOT NULL REFERENCES members(id),
    key text NOT NULL,
    post_id text NOT NULL REFERENCES posts(id),
    clap_id text REFERENCES claps(id),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (member_id, key)
);
CREATE INDEX idempotency_expiry ON idempotency_keys (member_id, created_at);
