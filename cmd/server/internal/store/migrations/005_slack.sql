-- Slack integration (design: nexus features/builtin-apps/peerbonus/slack-integration.md §3.4).
CREATE TABLE slack_integration (
    id smallint PRIMARY KEY CHECK (id = 1),
    team_id text,
    team_name text,
    bot_user_id text,
    enterprise_id text,
    verified_at timestamptz,
    verified_by text REFERENCES members(id),
    announce_channel_id text,
    announce_channel_name text,
    announce_posts boolean NOT NULL DEFAULT false,
    dm_recipient_on_post boolean NOT NULL DEFAULT true,
    give_from_slack boolean NOT NULL DEFAULT false,
    locale text NOT NULL DEFAULT 'ja' CHECK (locale IN ('ja', 'en')),
    last_error_code text,
    last_error_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO slack_integration (id) VALUES (1);

CREATE TABLE slack_deliveries (
    id bigserial PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('post_announce', 'post_dm', 'clap_update', 'post_delete')),
    post_id text NOT NULL REFERENCES posts(id),
    clap_id text REFERENCES claps(id),
    member_id text REFERENCES members(id),
    announce_id bigint REFERENCES slack_deliveries(id),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'sent', 'skipped', 'failed')),
    attempts integer NOT NULL DEFAULT 0,
    next_at timestamptz NOT NULL,
    claimed_until timestamptz,
    channel_id text,
    message_ts text,
    last_error text,
    created_at timestamptz NOT NULL,
    sent_at timestamptz
);
CREATE INDEX slack_deliveries_pending ON slack_deliveries (next_at) WHERE state = 'pending';
CREATE INDEX slack_deliveries_post ON slack_deliveries (post_id, kind);
-- SL2: at most one pending clap_update per post (claps coalesce).
CREATE UNIQUE INDEX slack_deliveries_clap_update_pending ON slack_deliveries (post_id) WHERE kind = 'clap_update' AND state = 'pending';

-- S9: Slack user id cache keyed by email; slack_lookup_at is the negative cache stamp.
ALTER TABLE members ADD COLUMN slack_user_id text;
ALTER TABLE members ADD COLUMN slack_lookup_at timestamptz;
CREATE UNIQUE INDEX members_slack_user_id ON members (slack_user_id) WHERE slack_user_id IS NOT NULL;
