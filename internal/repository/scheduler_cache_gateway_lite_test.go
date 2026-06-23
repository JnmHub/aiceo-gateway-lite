package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMergeGatewayLiteMirrorCredentialsPreservesLocalSecrets(t *testing.T) {
	existing := &service.Account{
		Credentials: map[string]any{
			"api_key":       "local-secret",
			"base_url":      "https://old.example/v1",
			"model_mapping": map[string]any{"old": "old"},
		},
	}
	incoming := map[string]any{
		"base_url":      "https://mimo.example/v1",
		"model_mapping": map[string]any{"mimo-v2.5-pro": "mimo-v2.5-pro"},
	}

	merged := mergeGatewayLiteMirrorCredentials(incoming, existing)

	require.Equal(t, "local-secret", merged["api_key"])
	require.Equal(t, "https://mimo.example/v1", merged["base_url"])
	require.Equal(t, map[string]any{"mimo-v2.5-pro": "mimo-v2.5-pro"}, merged["model_mapping"])
}

func TestGatewayLiteMirrorSnapshotAccountRequiresMirrorMarker(t *testing.T) {
	require.False(t, gatewayLiteMirrorSnapshotAccount(service.Account{}))
	require.True(t, gatewayLiteMirrorSnapshotAccount(service.Account{Extra: map[string]any{"gateway_upstream_mirror_id": int64(10)}}))
}
