// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package enrollment

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/agentstate"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
	agenttag "github.com/NVIDIA/fleet-intelligence-agent/internal/tag"
)

type fakeBackendClient struct {
	enrollSAK string
	enrollJWT string
	enrollErr error
}

type inMemorySeedState struct {
	tags map[string]string
}

func (f *fakeBackendClient) Enroll(_ context.Context, sakToken string) (string, error) {
	f.enrollSAK = sakToken
	return f.enrollJWT, f.enrollErr
}

func (f *fakeBackendClient) UpsertNode(context.Context, string, *backendclient.NodeUpsertRequest, string) error {
	return nil
}

func (f *fakeBackendClient) UpsertNodeTags(context.Context, string, *backendclient.NodeTagsUpsertRequest, string) error {
	return nil
}

func (f *fakeBackendClient) GetNonce(context.Context, string, string) (*backendclient.NonceResponse, error) {
	return nil, nil
}

func (f *fakeBackendClient) SubmitAttestation(context.Context, string, *backendclient.AttestationRequest, string) error {
	return nil
}

func (s *inMemorySeedState) GetBackendBaseURL(context.Context) (string, bool, error) {
	return "", false, nil
}

func (s *inMemorySeedState) SetBackendBaseURL(context.Context, string) error { return nil }

func (s *inMemorySeedState) GetJWT(context.Context) (string, bool, error) { return "", false, nil }

func (s *inMemorySeedState) SetJWT(context.Context, string) error { return nil }

func (s *inMemorySeedState) GetSAK(context.Context) (string, bool, error) { return "", false, nil }

func (s *inMemorySeedState) SetSAK(context.Context, string) error { return nil }

func (s *inMemorySeedState) GetNodeUUID(context.Context) (string, bool, error) { return "", false, nil }

func (s *inMemorySeedState) SetNodeUUID(context.Context, string) error { return nil }

func (s *inMemorySeedState) GetEnrollmentTime(context.Context) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func (s *inMemorySeedState) SetEnrollmentTime(context.Context, time.Time) error { return nil }

func (s *inMemorySeedState) GetTags(context.Context) (map[string]string, bool, error) {
	if s.tags == nil {
		return nil, false, nil
	}
	cloned := map[string]string{}
	for key, value := range s.tags {
		cloned[key] = value
	}
	return cloned, true, nil
}

func (s *inMemorySeedState) SetTags(_ context.Context, value map[string]string) error {
	s.tags = map[string]string{}
	for key, v := range value {
		s.tags[key] = v
	}
	return nil
}

func setTagEnvForTest(t *testing.T, nodeGroup, computeZone, customTags string) {
	t.Helper()
	t.Setenv(agenttag.EnvNodeGroup, nodeGroup)
	t.Setenv(agenttag.EnvComputeZone, computeZone)
	t.Setenv(agenttag.EnvCustomTags, customTags)
}

func TestEnrollWorkflow(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	client := &fakeBackendClient{enrollJWT: "jwt-token"}
	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		require.Equal(t, "https://example.com", rawBaseURL)
		return client, nil
	}

	syncCalled := false
	syncInventoryAfterEnroll = func(ctx context.Context, cfg *config.Config) error {
		syncCalled = true
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.NoError(t, err)
	require.Equal(t, "sak-token", client.enrollSAK)
	require.True(t, syncCalled)
}

func TestEnrollWorkflowPassesConfigToInventorySync(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}

	wantCfg := &config.Config{
		Inventory: &config.InventoryConfig{
			Enabled: true,
		},
	}
	syncInventoryAfterEnroll = func(ctx context.Context, gotCfg *config.Config) error {
		require.Same(t, wantCfg, gotCfg)
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := EnrollWithConfig(context.Background(), "https://example.com", "sak-token", wantCfg)
	require.NoError(t, err)
}

func TestEnrollWorkflowDefaultWrapperUsesNilConfig(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}

	syncInventoryAfterEnroll = func(ctx context.Context, gotCfg *config.Config) error {
		require.Nil(t, gotCfg)
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.NoError(t, err)
}

func TestEnrollWorkflowNormalizesLegacyEndpointToBaseURL(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	client := &fakeBackendClient{enrollJWT: "jwt-token"}
	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		require.Equal(t, "https://example.com", rawBaseURL)
		return client, nil
	}
	syncInventoryAfterEnroll = func(context.Context, *config.Config) error { return nil }

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Enroll(context.Background(), "https://example.com/api/v1/health/metrics", "sak-token")
	require.NoError(t, err)
	require.Equal(t, "sak-token", client.enrollSAK)
}

func TestEnrollWorkflowErrors(t *testing.T) {
	t.Run("invalid endpoint", func(t *testing.T) {
		err := Enroll(context.Background(), "http://example.com", "sak-token")
		require.Error(t, err)
	})

	t.Run("localhost http endpoint allowed", func(t *testing.T) {
		originalFactory := newBackendClient
		t.Cleanup(func() { newBackendClient = originalFactory })

		called := false
		newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
			called = true
			require.Equal(t, "http://localhost:8080", rawBaseURL)
			return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
		}

		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		err := Enroll(context.Background(), "http://localhost:8080", "sak-token")
		require.NoError(t, err)
		require.True(t, called)
	})

	t.Run("backend client creation", func(t *testing.T) {
		originalFactory := newBackendClient
		t.Cleanup(func() { newBackendClient = originalFactory })
		newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
			return nil, errors.New("factory boom")
		}

		err := Enroll(context.Background(), "https://example.com", "sak-token")
		require.ErrorContains(t, err, "failed to create backend client")
	})

	t.Run("enroll error", func(t *testing.T) {
		originalFactory := newBackendClient
		t.Cleanup(func() { newBackendClient = originalFactory })
		newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
			return &fakeBackendClient{enrollErr: errors.New("enroll boom")}, nil
		}

		err := Enroll(context.Background(), "https://example.com", "sak-token")
		require.ErrorContains(t, err, "enroll boom")
	})

	t.Run("localhost legacy endpoint allowed", func(t *testing.T) {
		originalFactory := newBackendClient
		originalSync := syncInventoryAfterEnroll
		t.Cleanup(func() {
			newBackendClient = originalFactory
			syncInventoryAfterEnroll = originalSync
		})

		called := false
		newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
			called = true
			require.Equal(t, "http://localhost:8080", rawBaseURL)
			return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
		}
		syncInventoryAfterEnroll = func(context.Context, *config.Config) error { return nil }

		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		err := Enroll(context.Background(), "http://localhost:8080/api/v1/health/enroll", "sak-token")
		require.NoError(t, err)
		require.True(t, called)
	})
}

func TestEnrollWorkflowInventorySyncFailureIsNonFatal(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}
	syncInventoryAfterEnroll = func(ctx context.Context, cfg *config.Config) error {
		return errors.New("inventory failed")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.NoError(t, err)
}

func TestEnrollWorkflowInventorySyncTimeoutIsNonFatal(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	originalTimeout := postEnrollInventorySyncTimeout
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
		postEnrollInventorySyncTimeout = originalTimeout
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}
	postEnrollInventorySyncTimeout = 10 * time.Millisecond

	syncStarted := make(chan struct{})
	releaseSync := make(chan struct{})
	syncInventoryAfterEnroll = func(context.Context, *config.Config) error {
		close(syncStarted)
		<-releaseSync
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	start := time.Now()
	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.NoError(t, err)
	require.Less(t, time.Since(start), time.Second)

	select {
	case <-syncStarted:
	case <-time.After(time.Second):
		t.Fatal("inventory sync did not start")
	}
	close(releaseSync)
}

func TestEnrollWorkflowInventorySyncUsesRemainingEnrollTimeout(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	originalTimeout := postEnrollInventorySyncTimeout
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
		postEnrollInventorySyncTimeout = originalTimeout
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}
	postEnrollInventorySyncTimeout = time.Minute

	syncInventoryAfterEnroll = func(ctx context.Context, cfg *config.Config) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Less(t, time.Until(deadline), 5*time.Second)
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Enroll(ctx, "https://example.com", "sak-token")
	require.NoError(t, err)
}

func TestEnrollWorkflowSeedsTagsFromEnv(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	setTagEnvForTest(t, "group-a", "zone-a", "owner=ml-platform")
	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}
	syncInventoryAfterEnroll = func(context.Context, *config.Config) error { return nil }

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.NoError(t, err)

	tags, ok, err := agentstate.NewSQLite().GetTags(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		agenttag.ReservedKeyNodeGroup:   "group-a",
		agenttag.ReservedKeyComputeZone: "zone-a",
		"owner":                         "ml-platform",
	}, tags)
}

func TestEnrollWorkflowRejectsComputeZoneWithoutNodeGroupFromEnv(t *testing.T) {
	setTagEnvForTest(t, "", "zone-a", "")
	err := EnrollWithConfig(context.Background(), "https://example.com", "sak-token", nil)
	require.ErrorContains(t, err, agenttag.ReservedKeyNodeGroup)
}

func TestEnrollWorkflowRejectsNodeGroupWithoutComputeZoneFromEnv(t *testing.T) {
	setTagEnvForTest(t, "group-a", "", "")
	err := EnrollWithConfig(context.Background(), "https://example.com", "sak-token", nil)
	require.ErrorContains(t, err, agenttag.ReservedKeyComputeZone)
}

func TestEnrollWorkflowPreservesExistingTagsOnReenroll(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}
	syncInventoryAfterEnroll = func(context.Context, *config.Config) error { return nil }

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := agentstate.NewSQLite().SetTags(context.Background(), map[string]string{
		agenttag.ReservedKeyNodeGroup:   "group-from-db",
		agenttag.ReservedKeyComputeZone: "zone-from-db",
		"owner":                         "team-a",
	})
	require.NoError(t, err)

	setTagEnvForTest(t, "group-from-env", "zone-from-env", "owner=team-b,stack=prod")
	err = Enroll(context.Background(), "https://example.com", "sak-token")
	require.NoError(t, err)

	tags, ok, err := agentstate.NewSQLite().GetTags(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		agenttag.ReservedKeyNodeGroup:   "group-from-db",
		agenttag.ReservedKeyComputeZone: "zone-from-db",
		"owner":                         "team-a",
		"stack":                         "prod",
	}, tags)
}

func TestSeedTagsInMetadataReturnsPatchOnlyForMissingKeys(t *testing.T) {
	state := &inMemorySeedState{
		tags: map[string]string{
			"owner": "team-a",
		},
	}

	patch, err := seedTagsInMetadata(context.Background(), state, map[string]string{
		"owner": "team-b",
		"stack": "prod",
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"stack": "prod",
	}, patch)
	require.Equal(t, map[string]string{
		"owner": "team-a",
		"stack": "prod",
	}, state.tags)
}

func TestToNodeTagsUpsertRequestBuildsStructuredDTO(t *testing.T) {
	req := toNodeTagsUpsertRequest(map[string]string{
		agenttag.ReservedKeyNodeGroup:   "group-a",
		agenttag.ReservedKeyComputeZone: "",
		"owner":                         "team-a",
		"stack":                         "",
	})
	require.NotNil(t, req.NodeGroup)
	require.Equal(t, "group-a", *req.NodeGroup)
	require.NotNil(t, req.ComputeZone)
	require.Equal(t, "", *req.ComputeZone)
	require.Equal(t, map[string]string{"owner": "team-a"}, req.CustomSet)
	require.Equal(t, []string{"stack"}, req.CustomRemove)
}

func TestToNodeTagsUpsertRequestOmitsUnsetReservedFields(t *testing.T) {
	req := toNodeTagsUpsertRequest(map[string]string{
		"owner": "team-a",
	})
	require.Nil(t, req.NodeGroup)
	require.Nil(t, req.ComputeZone)
	require.Equal(t, map[string]string{"owner": "team-a"}, req.CustomSet)
	require.Empty(t, req.CustomRemove)
}

func TestStoreConfigInMetadataSecuresFreshStateFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test expects non-root default state path resolution")
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := storeConfigInMetadata(
		context.Background(),
		"https://example.com",
		"jwt-token",
		"sak-token",
		time.Now().UTC(),
	)
	require.NoError(t, err)

	stateFile, err := config.DefaultStateFile()
	require.NoError(t, err)
	for _, candidate := range []string{stateFile, stateFile + "-wal", stateFile + "-shm"} {
		info, err := os.Stat(candidate)
		if os.IsNotExist(err) {
			if candidate == stateFile {
				require.NoError(t, err)
			}
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}
