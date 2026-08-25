CREATE TABLE IF NOT EXISTS users (
    id                   VARCHAR(36)  PRIMARY KEY,
    email                VARCHAR(255) UNIQUE,
    username             VARCHAR(50)  NOT NULL UNIQUE,
    password_hash        VARCHAR(255),
    is_admin             BOOLEAN      NOT NULL DEFAULT FALSE,
    must_change_password BOOLEAN      NOT NULL DEFAULT FALSE,
    avatar               VARCHAR(50)  NOT NULL DEFAULT 'default',
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_seen            TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ix_users_email    ON users (email);
CREATE INDEX IF NOT EXISTS ix_users_username ON users (username);
CREATE INDEX IF NOT EXISTS ix_users_is_admin ON users (is_admin);

CREATE TABLE IF NOT EXISTS custom_emojis (
    id            VARCHAR(36)  PRIMARY KEY,
    name          VARCHAR(50)  NOT NULL UNIQUE,
    url           VARCHAR(500) NOT NULL,
    object_name   VARCHAR(255),
    created_by_id VARCHAR(36)  REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ix_custom_emojis_name ON custom_emojis (name);

CREATE TABLE IF NOT EXISTS messages (
    id          VARCHAR(36) PRIMARY KEY,
    content     TEXT,
    author_id   VARCHAR(36) NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reply_to_id VARCHAR(36) REFERENCES messages (id) ON DELETE SET NULL,
    is_pinned   BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ,
    edited_at   TIMESTAMPTZ,
    attachments JSON        NOT NULL DEFAULT '[]'::json
);

CREATE INDEX IF NOT EXISTS ix_messages_author_id          ON messages (author_id);
CREATE INDEX IF NOT EXISTS ix_messages_reply_to_id        ON messages (reply_to_id);
CREATE INDEX IF NOT EXISTS ix_messages_is_pinned          ON messages (is_pinned);
CREATE INDEX IF NOT EXISTS ix_messages_created_at         ON messages (created_at);
CREATE INDEX IF NOT EXISTS ix_messages_created_at_desc    ON messages (created_at DESC);
CREATE INDEX IF NOT EXISTS ix_messages_pinned_created     ON messages (is_pinned, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_messages_keyset             ON messages (created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS reactions (
    id              VARCHAR(36) PRIMARY KEY,
    message_id      VARCHAR(36) NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    user_id         VARCHAR(36) NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    emoji           VARCHAR(50) NOT NULL,
    custom_emoji_id VARCHAR(36) REFERENCES custom_emojis (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ix_reactions_message_id ON reactions (message_id);
CREATE INDEX IF NOT EXISTS ix_reactions_user_id    ON reactions (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS ix_reactions_message_user_emoji
    ON reactions (message_id, user_id, emoji);

CREATE TABLE IF NOT EXISTS fcm_tokens (
    id         VARCHAR(36)  PRIMARY KEY,
    token      VARCHAR(512) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ix_fcm_tokens_token ON fcm_tokens (token);

ALTER TABLE messages      ADD COLUMN IF NOT EXISTS edited_at       TIMESTAMPTZ;
ALTER TABLE messages      ADD COLUMN IF NOT EXISTS updated_at      TIMESTAMPTZ;
ALTER TABLE messages      ADD COLUMN IF NOT EXISTS is_pinned       BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE reactions     ADD COLUMN IF NOT EXISTS custom_emoji_id VARCHAR(36);
ALTER TABLE custom_emojis ADD COLUMN IF NOT EXISTS object_name     VARCHAR(255);

DO $$
DECLARE
    len integer;
BEGIN
    SELECT character_maximum_length INTO len
      FROM information_schema.columns
     WHERE table_name = 'reactions' AND column_name = 'emoji';
    IF len IS NOT NULL AND len < 50 THEN
        ALTER TABLE reactions ALTER COLUMN emoji TYPE VARCHAR(50);
    END IF;
END
$$;

DO $$
DECLARE
    col smallint;
BEGIN
    SELECT attnum INTO col
      FROM pg_attribute
     WHERE attrelid = 'reactions'::regclass
       AND attname = 'custom_emoji_id'
       AND NOT attisdropped;

    IF col IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM pg_constraint
         WHERE conrelid = 'reactions'::regclass
           AND contype = 'f'
           AND conkey = ARRAY[col]
    ) THEN
        ALTER TABLE reactions
            ADD CONSTRAINT fk_reactions_custom_emoji_id
            FOREIGN KEY (custom_emoji_id) REFERENCES custom_emojis (id) ON DELETE SET NULL;
    END IF;
END
$$;
