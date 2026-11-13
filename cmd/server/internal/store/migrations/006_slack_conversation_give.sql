-- Giving inside a Slack conversation: bot mentions and admin-chosen reactions
-- (design: nexus features/peerbonus-slack/reaction-give.md §3).
ALTER TABLE slack_integration ADD COLUMN give_in_conversation boolean NOT NULL DEFAULT false;
ALTER TABLE slack_integration ADD COLUMN mention_point_choices integer[] NOT NULL DEFAULT '{10,30,50}';

CREATE TABLE slack_reaction_rules (
    emoji text PRIMARY KEY,
    points integer NOT NULL CHECK (points > 0),
    value_key text,
    position smallint NOT NULL
);

-- One give per (message, sender, recipient), replacing reaction-give.md's per-emoji RL2.
-- Emoji is recorded for reference only; emoji = '' is a mention.
-- The row outlives idempotency_keys, so removing and re-adding a reaction never gives twice.
CREATE TABLE slack_gives (
    channel_id text NOT NULL,
    message_ts text NOT NULL,
    sender_id text NOT NULL REFERENCES members(id),
    recipient_id text NOT NULL REFERENCES members(id),
    emoji text NOT NULL DEFAULT '',
    post_id text NOT NULL REFERENCES posts(id),
    clap_id text REFERENCES claps(id),
    permalink text,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (channel_id, message_ts, sender_id, recipient_id)
);
CREATE INDEX slack_gives_post ON slack_gives (post_id);

-- R8: a mention without points waits for the sender to pick an amount.
CREATE TABLE slack_pending_gives (
    id text PRIMARY KEY,
    channel_id text NOT NULL,
    message_ts text NOT NULL,
    thread_ts text,
    sender_id text NOT NULL REFERENCES members(id),
    recipient_ids text[] NOT NULL,
    message text NOT NULL,
    expires_at timestamptz NOT NULL,
    UNIQUE (channel_id, message_ts)
);
