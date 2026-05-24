-- name: CreateAccount :one
INSERT INTO accounts (id, email, display_name, github_user_id, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAccountByID :one
SELECT id, email, display_name, github_user_id, created_at, is_anonymous
FROM accounts
WHERE id = ?;

-- name: GetAccountByEmail :one
SELECT id, email, display_name, github_user_id, created_at, is_anonymous
FROM accounts
WHERE email = ?;

-- name: GetAccountByGitHubUserID :one
SELECT id, email, display_name, github_user_id, created_at, is_anonymous
FROM accounts
WHERE github_user_id = ?;

-- name: UpdateAccountDisplayName :exec
UPDATE accounts
SET display_name = ?
WHERE id = ?;

-- name: CreateAnonymousAccount :one
-- Creates an anonymous account for a playground session participant.
-- The synthetic email satisfies the NOT NULL UNIQUE constraint without
-- requiring schema relaxation; the @playground.local suffix and the
-- random ID prefix guarantee uniqueness.
INSERT INTO accounts (id, email, display_name, github_user_id, created_at, is_anonymous)
VALUES (?, ?, ?, NULL, ?, 1)
RETURNING *;
