package auth

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/db/store"
)

// SlugFromName derives a URL-safe slug from a human-readable name. It
// lowercases the name, replaces runs of non-alphanumeric characters with
// hyphens, and trims leading/trailing hyphens.
func SlugFromName(name string) string {
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(strings.ToLower(name), "-")
	return strings.Trim(slug, "-")
}

// CreateOrgWithSlug creates an org row derived from name, retrying with a
// random 6-char suffix on unique-constraint violations. It is shared by the
// auto-provisioning path (provision.go) and the manual org-creation handler
// (accounts/handlers.go).
func CreateOrgWithSlug(ctx context.Context, s store.OrgStore, name string, now time.Time) (store.Org, error) {
	baseSlug := SlugFromName(name)

	// First attempt: clean slug.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      baseSlug,
		CreatedAt: now,
	})
	if err == nil {
		return org, nil
	}
	if !errors.Is(err, store.ErrUniqueViolation) {
		return store.Org{}, fmt.Errorf("auth: create org: %w", err)
	}

	// Collision: append a random 6-char alphanumeric suffix.
	slug := baseSlug + "-" + randomSuffix(6)
	org, err = s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        uuid.New().String(),
		Name:      name,
		Slug:      slug,
		CreatedAt: now,
	})
	if err != nil {
		return store.Org{}, fmt.Errorf("auth: create org (retry): %w", err)
	}
	return org, nil
}

const alphanumChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// randomSuffix returns n random lowercase alphanumeric characters.
func randomSuffix(n int) string {
	// rand.Intn is fine for non-secret slug disambiguation.
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanumChars[rng.Intn(len(alphanumChars))]
	}
	return string(b)
}
