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

	pkgmetadata "github.com/NVIDIA/fleet-intelligence-sdk/pkg/metadata"
	"github.com/NVIDIA/fleet-intelligence-sdk/pkg/sqlite"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/config"
)

type fakeBackendClient struct {
	enrollSAK      string
	enrollJWT      string
	enrollErr      error
	upsertNodeUUID string
	upsertJWT      string
	upsertReq      *backendclient.NodeUpsertRequest
	upsertErr      error
}

func (f *fakeBackendClient) Enroll(_ context.Context, sakToken string) (string, error) {
	f.enrollSAK = sakToken
	return f.enrollJWT, f.enrollErr
}

func (f *fakeBackendClient) UpsertNode(_ context.Context, nodeUUID string, req *backendclient.NodeUpsertRequest, jwt string) error {
	f.upsertNodeUUID = nodeUUID
	f.upsertReq = req
	f.upsertJWT = jwt
	return f.upsertErr
}

func (f *fakeBackendClient) GetNonce(context.Context, string, string) (*backendclient.NonceResponse, error) {
	return nil, nil
}

func (f *fakeBackendClient) SubmitAttestation(context.Context, string, *backendclient.AttestationRequest, string) error {
	return nil
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
	syncInventoryAfterEnroll = func(ctx context.Context, gotClient backendclient.Client, nodeUUID, jwt string, cfg *config.Config) error {
		syncCalled = true
		require.Same(t, client, gotClient)
		require.NotEmpty(t, nodeUUID)
		require.Equal(t, "jwt-token", jwt)
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
	syncInventoryAfterEnroll = func(ctx context.Context, gotClient backendclient.Client, nodeUUID, jwt string, gotCfg *config.Config) error {
		require.NotNil(t, gotClient)
		require.NotEmpty(t, nodeUUID)
		require.Equal(t, "jwt-token", jwt)
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

	syncInventoryAfterEnroll = func(ctx context.Context, gotClient backendclient.Client, nodeUUID, jwt string, gotCfg *config.Config) error {
		require.NotNil(t, gotClient)
		require.NotEmpty(t, nodeUUID)
		require.Equal(t, "jwt-token", jwt)
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
	syncInventoryAfterEnroll = func(context.Context, backendclient.Client, string, string, *config.Config) error { return nil }

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
		syncInventoryAfterEnroll = func(context.Context, backendclient.Client, string, string, *config.Config) error { return nil }

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
		syncInventoryAfterEnroll = func(context.Context, backendclient.Client, string, string, *config.Config) error { return nil }

		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		err := Enroll(context.Background(), "http://localhost:8080/api/v1/health/enroll", "sak-token")
		require.NoError(t, err)
		require.True(t, called)
	})
}

func TestEnrollWorkflowInventorySyncFailureIsFatal(t *testing.T) {
	originalFactory := newBackendClient
	originalSync := syncInventoryAfterEnroll
	originalStore := storeEnrollmentConfig
	t.Cleanup(func() {
		newBackendClient = originalFactory
		syncInventoryAfterEnroll = originalSync
		storeEnrollmentConfig = originalStore
	})

	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		return &fakeBackendClient{enrollJWT: "jwt-token"}, nil
	}
	syncInventoryAfterEnroll = func(ctx context.Context, gotClient backendclient.Client, nodeUUID, jwt string, cfg *config.Config) error {
		require.NotNil(t, gotClient)
		require.NotEmpty(t, nodeUUID)
		require.Equal(t, "jwt-token", jwt)
		return errors.New("inventory failed")
	}
	storeCalled := false
	storeEnrollmentConfig = func(context.Context, string, string, string, string) error {
		storeCalled = true
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.ErrorContains(t, err, "initial node upsert failed")
	require.False(t, storeCalled, "enroll should not persist active metadata when initial node upsert fails")
}

func TestEnrollWorkflowInventorySyncTimeoutIsFatal(t *testing.T) {
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
	syncInventoryAfterEnroll = func(context.Context, backendclient.Client, string, string, *config.Config) error {
		close(syncStarted)
		<-releaseSync
		return nil
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	start := time.Now()
	err := Enroll(context.Background(), "https://example.com", "sak-token")
	require.ErrorContains(t, err, "initial node upsert failed")
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

	syncInventoryAfterEnroll = func(ctx context.Context, gotClient backendclient.Client, nodeUUID, jwt string, cfg *config.Config) error {
		require.NotNil(t, gotClient)
		require.NotEmpty(t, nodeUUID)
		require.Equal(t, "jwt-token", jwt)
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
		"550e8400-e29b-41d4-a716-446655440000",
	)
	require.NoError(t, err)

	stateFile, err := config.DefaultStateFile()
	require.NoError(t, err)
	db, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	require.NoError(t, err)
	defer db.Close()
	machineID, err := pkgmetadata.ReadMachineID(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", machineID)

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
