-- 0001_init.sql: accounts, economy, hands, hand_players.

CREATE TABLE IF NOT EXISTS accounts (
    player_id  uuid PRIMARY KEY,
    email      text UNIQUE,
    guest      boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- player_id is text (not a FK to accounts) so the economy store can be
-- exercised standalone, without coupling to account creation order.
CREATE TABLE IF NOT EXISTS economy (
    player_id   text PRIMARY KEY,
    balance     bigint NOT NULL,
    last_refill timestamptz,
    streak      integer NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS hands (
    hand_id    text PRIMARY KEY,
    table_id   text NOT NULL,
    started_at timestamptz NOT NULL,
    record     jsonb NOT NULL
);

-- started_at is denormalized from hands so ByPlayer can order/filter without
-- a join hitting the (much larger) hands table's jsonb column.
CREATE TABLE IF NOT EXISTS hand_players (
    hand_id    text NOT NULL REFERENCES hands (hand_id) ON DELETE CASCADE,
    player_id  text NOT NULL,
    started_at timestamptz NOT NULL,
    PRIMARY KEY (hand_id, player_id)
);

CREATE INDEX IF NOT EXISTS idx_hand_players_player_started
    ON hand_players (player_id, started_at DESC);
