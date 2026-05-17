-- name: CreateAccount :one
INSERT INTO accounts (id, email, display_name, github_user_id, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAccountByID :one
SELECT id, email, display_name, github_user_id, created_at
FROM accounts
WHERE id = $1;

-- name: GetAccountByEmail :one
SELECT id, email, display_name, github_user_id, created_at
FROM accounts
WHERE email = $1;

-- name: GetAccountByGitHubUserID :one
SELECT id, email, display_name, github_user_id, created_at
FROM accounts
WHERE github_user_id = $1;

-- name: UpdateAccountDisplayName :exec
UPDATE accounts
SET display_name = $1
WHERE id = $2;
