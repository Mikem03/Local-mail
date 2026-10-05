package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimEmails_ValidatesBatchLimit(t *testing.T) {
	store := &Postgres{}
	invalidLimits := []struct {
		name  string
		limit int
	}{
		{name: "zero", limit: 0},
		{name: "negative", limit: -1},
	}
	for _, testCase := range invalidLimits {
		t.Run(testCase.name, func(t *testing.T) {
			emails, err := store.ClaimEmails(context.Background(), testCase.limit, time.Minute)

			require.Error(t, err)
			assert.Nil(t, emails)
			assert.ErrorContains(t, err, "claim limit must be positive")
		})
	}
}

func TestClaimEmails_ValidatesLeaseDuration(t *testing.T) {
	invalidLeases := []struct {
		name  string
		lease time.Duration
	}{
		{name: "zero", lease: 0},
		{name: "negative", lease: -time.Second},
		{name: "less than one millisecond", lease: time.Microsecond},
	}

	store := &Postgres{}
	for _, testCase := range invalidLeases {
		t.Run(testCase.name, func(t *testing.T) {
			emails, err := store.ClaimEmails(context.Background(), 1, testCase.lease)

			require.Error(t, err)
			assert.Nil(t, emails)
			assert.ErrorContains(t, err, "lease duration must be positive")
		})
	}
}

func TestCompleteEmail_RejectsMalformedRawResponseBeforeDatabaseAccess(t *testing.T) {
	store := &Postgres{}
	err := store.CompleteEmail(context.Background(), 42, time.Now(), Extraction{
		RawResponse: json.RawMessage(`{"company":`),
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, "raw extraction response must be valid JSON")
}

func TestReleaseEmail_RequiresFailureCauseBeforeDatabaseAccess(t *testing.T) {
	store := &Postgres{}
	err := store.ReleaseEmail(context.Background(), 42, time.Now(), nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "release cause must not be nil")
	assert.False(t, errors.Is(err, ErrLeaseLost))
}
