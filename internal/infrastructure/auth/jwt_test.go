package auth_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lauvale029/Prueba_tecnica_GO-Python/internal/infrastructure/auth"
)

func TestIssueAndValidateToken_RoundTrip(t *testing.T) {
	token, expiresAt, err := auth.IssueToken("un-secreto", "mova-service", time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.WithinDuration(t, time.Now().UTC().Add(time.Hour), expiresAt, time.Second)

	subject, err := auth.ValidateToken("un-secreto", token)
	require.NoError(t, err)
	require.Equal(t, "mova-service", subject)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, _, err := auth.IssueToken("un-secreto", "mova-service", time.Hour)
	require.NoError(t, err)

	_, err = auth.ValidateToken("otro-secreto", token)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateToken_Expired(t *testing.T) {
	token, _, err := auth.IssueToken("un-secreto", "mova-service", -time.Minute)
	require.NoError(t, err)

	_, err = auth.ValidateToken("un-secreto", token)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := auth.ValidateToken("un-secreto", "esto-no-es-un-jwt")
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}