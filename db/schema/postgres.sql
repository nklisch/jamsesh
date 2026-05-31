CREATE TABLE orgs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    session_invite_policy TEXT NOT NULL DEFAULT 'members_only'
        CHECK (session_invite_policy IN ('members_only', 'open')),
    org_protected BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    github_user_id TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    is_anonymous BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE org_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (org_id, account_id)
);
CREATE INDEX org_members_account_idx ON org_members(account_id);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT NOT NULL,
    writable_scope TEXT NOT NULL,
    default_mode TEXT NOT NULL CHECK (default_mode IN ('sync','isolated')),
    base_sha TEXT,
    status TEXT NOT NULL CHECK (status IN ('active','finalizing','ended','archived')),
    created_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    end_reason TEXT,
    finalize_locked_by_account_id TEXT REFERENCES accounts(id),
    last_substantive_activity_at TIMESTAMPTZ,  -- NOT NULL for new rows; nullable only for pre-migration rows
    hard_cap_at TIMESTAMPTZ,                   -- nullable; set only for playground sessions
    idle_timeout_at TIMESTAMPTZ                -- nullable; set only for playground sessions
);
CREATE INDEX sessions_org_idx ON sessions(org_id);

CREATE TABLE session_members (
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('creator','member')),
    joined_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (session_id, account_id)
);
CREATE INDEX session_members_org_idx ON session_members(org_id);
CREATE INDEX session_members_account_idx ON session_members(account_id);

CREATE TABLE oauth_tokens (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    kind TEXT NOT NULL CHECK (kind IN ('access','refresh','anonymous_session_bearer')),
    session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX oauth_tokens_account_idx ON oauth_tokens(account_id);
CREATE INDEX oauth_tokens_session_idx ON oauth_tokens(session_id)
  WHERE session_id IS NOT NULL;

CREATE TABLE magic_link_tokens (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);

CREATE TABLE resume_tokens (
    id          TEXT PRIMARY KEY,
    token_hash  TEXT NOT NULL UNIQUE,
    session_id  TEXT NOT NULL,
    org_id      TEXT NOT NULL,
    account_id  TEXT NOT NULL,
    issued_at   TIMESTAMPTZ NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);

CREATE TABLE archived_sessions (
    session_id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal_text TEXT NOT NULL,
    member_account_ids TEXT NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    archived_at TIMESTAMPTZ NOT NULL,
    end_reason TEXT NOT NULL CHECK (end_reason IN ('finalize','abandon','timeout')),
    final_branch_name TEXT
);
CREATE INDEX archived_sessions_org_idx ON archived_sessions(org_id);

CREATE TABLE oauth_state (
    nonce TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX oauth_state_expires_idx ON oauth_state(expires_at);

-- ---------------------------------------------------------------------------
-- Event log tables (00004_events migration)
-- ---------------------------------------------------------------------------

CREATE TABLE event_seq (
    session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    next BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE events (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    type TEXT NOT NULL,
    payload TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(session_id, seq)
);
CREATE INDEX events_session_created_idx ON events(session_id, created_at);
CREATE INDEX events_org_idx ON events(org_id);

CREATE TABLE presence (
    org_id TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    current_sha TEXT NOT NULL,
    last_active_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (session_id, account_id, ref)
);
CREATE INDEX presence_org_idx ON presence(org_id);

-- ---------------------------------------------------------------------------
-- Org invites table (00005_org_invites migration)
-- ---------------------------------------------------------------------------

CREATE TABLE org_invites (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    inviter_account_id TEXT NOT NULL REFERENCES accounts(id),
    recipient_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    accepted_by_account_id TEXT REFERENCES accounts(id)
);
CREATE INDEX org_invites_org_idx ON org_invites(org_id);
CREATE INDEX org_invites_email_idx ON org_invites(recipient_email);

-- ---------------------------------------------------------------------------
-- ref_modes table (00006_sessions_lifecycle migration)
-- ---------------------------------------------------------------------------

CREATE TABLE ref_modes (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    ref TEXT NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('sync','isolated')),
    PRIMARY KEY (session_id, ref)
);

-- ---------------------------------------------------------------------------
-- session_invites table (00007_session_invites migration)
-- ---------------------------------------------------------------------------

CREATE TABLE session_invites (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    inviter_account_id TEXT NOT NULL REFERENCES accounts(id),
    invitee_email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    accepted_by_account_id TEXT REFERENCES accounts(id)
);
CREATE INDEX session_invites_session_idx ON session_invites(session_id);
CREATE INDEX session_invites_email_idx ON session_invites(invitee_email);

-- ---------------------------------------------------------------------------
-- conflict_events table (00008_conflict_events migration)
-- ---------------------------------------------------------------------------

CREATE TABLE conflict_events (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    source_commit TEXT NOT NULL,
    draft_tip TEXT NOT NULL,
    ancestor TEXT NOT NULL,
    conflicts TEXT NOT NULL,     -- JSON
    addressed_to TEXT NOT NULL,  -- JSON
    status TEXT NOT NULL CHECK (status IN ('open','resolved')),
    resolving_commit_sha TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ
);
CREATE INDEX conflict_events_session_status_idx ON conflict_events(session_id, status);

-- ---------------------------------------------------------------------------
-- comments table (00009_comments migration)
-- ---------------------------------------------------------------------------

CREATE TABLE comments (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    author_account_id TEXT NOT NULL REFERENCES accounts(id),
    author_kind TEXT NOT NULL CHECK (author_kind IN ('human','agent')),
    anchor_commit_sha TEXT NOT NULL,
    anchor_file_path TEXT,
    anchor_line_start INTEGER,
    anchor_line_end INTEGER,
    body TEXT NOT NULL,
    addressed_to TEXT,
    kind TEXT NOT NULL CHECK (kind IN ('question','suggestion','action-request','fyi')),
    created_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolved_by_account_id TEXT REFERENCES accounts(id),
    resolution_note TEXT
);
CREATE INDEX comments_session_idx ON comments(session_id, created_at);
CREATE INDEX comments_addressed_idx ON comments(addressed_to);

-- ---------------------------------------------------------------------------
-- finalize_locks table (00010_finalize_locks migration)
-- ---------------------------------------------------------------------------

CREATE TABLE finalize_locks (
    id TEXT PRIMARY KEY,
    org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    acquired_by_account_id TEXT NOT NULL REFERENCES accounts(id),
    acquired_at TIMESTAMPTZ NOT NULL,
    last_activity_at TIMESTAMPTZ NOT NULL,
    selected_commit_shas JSONB NOT NULL DEFAULT '[]'::jsonb,
    target_branch TEXT NOT NULL DEFAULT '',
    base_sha TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL DEFAULT 'squash'
        CHECK (mode IN ('squash','preserve')),
    commit_message TEXT,
    superseded_by_lock_id TEXT REFERENCES finalize_locks(id),
    released_at TIMESTAMPTZ
);
CREATE INDEX finalize_locks_session_idx ON finalize_locks(session_id);
CREATE INDEX finalize_locks_active_idx ON finalize_locks(session_id)
    WHERE released_at IS NULL AND superseded_by_lock_id IS NULL;
-- Unique partial index (00015_finalize_locks_unique_active): at most one active
-- (non-superseded, non-released) lock may exist per session at any time.
CREATE UNIQUE INDEX finalize_locks_one_active_per_session_idx
    ON finalize_locks (session_id)
    WHERE superseded_by_lock_id IS NULL AND released_at IS NULL;

-- ---------------------------------------------------------------------------
-- leases table (00013_leases migration)
-- ---------------------------------------------------------------------------

CREATE SEQUENCE jamsesh_lease_fencing_tokens AS bigint;

CREATE TABLE leases (
    session_id     TEXT        PRIMARY KEY,
    pod_id         TEXT        NOT NULL,
    fencing_token  BIGINT      NOT NULL,
    acquired_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_at    TIMESTAMPTZ,
    heartbeat_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX leases_released_at_idx ON leases(released_at) WHERE released_at IS NOT NULL;

-- ---------------------------------------------------------------------------
-- tombstones table (00018_playground_sessions migration)
-- ---------------------------------------------------------------------------

CREATE TABLE tombstones (
    session_id         TEXT PRIMARY KEY,
    org_id             TEXT NOT NULL,
    members_count      INTEGER NOT NULL,
    commits_count      INTEGER NOT NULL,
    auto_merges_count  INTEGER NOT NULL,
    duration_seconds   INTEGER NOT NULL,
    end_reason         TEXT NOT NULL,
    ended_at           TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL
);
CREATE INDEX tombstones_expires_idx ON tombstones(expires_at);
