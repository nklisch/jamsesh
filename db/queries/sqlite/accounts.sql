-- name: CreateAccount :one
INSERT INTO accounts (id, email, display_name, github_user_id, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: GetAccountByID :one
SELECT id, email, display_name, github_user_id, created_at
FROM accounts
WHERE id = ?;

-- name: GetAccountByEmail :one
SELECT id, email, display_name, github_user_id, created_at
FROM accounts
WHERE email = ?;

-- name: GetAccountByGitHubUserID :one
SELECT id, email, display_name, github_user_id, created_at
FROM accounts
WHERE github_user_id = ?;

-- name: UpdateAccountDisplayName :exec
UPDATE accounts
SET display_name = ?
WHERE id = ?;
