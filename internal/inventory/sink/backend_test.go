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

package sink

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/inventory"
)

type fakeState struct {
	baseURL  string
	jwt      string
	nodeUUID string
	err      error
}

func (f fakeState) GetBackendBaseURL(context.Context) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	return f.baseURL, f.baseURL != "", nil
}
func (f fakeState) SetBackendBaseURL(context.Context, string) error { return nil }
func (f fakeState) GetJWT(context.Context) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	return f.jwt, f.jwt != "", nil
}
func (f fakeState) SetJWT(context.Context, string) error         { return nil }
func (f fakeState) GetSAK(context.Context) (string, bool, error) { return "", false, nil }
func (f fakeState) SetSAK(context.Context, string) error         { return nil }
func (f fakeState) GetNodeUUID(context.Context) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	return f.nodeUUID, f.nodeUUID != "", nil
}
func (f fakeState) SetNodeUUID(context.Context, string) error { return nil }

type fakeClient struct {
	nodeUUID string
	req      *backendclient.NodeUpsertRequest
	jwt      string
}

func (f *fakeClient) Enroll(context.Context, string) (string, error) { return "", nil }
func (f *fakeClient) GetNonce(context.Context, string, string) (*backendclient.NonceResponse, error) {
	return nil, nil
}
func (f *fakeClient) SubmitAttestation(context.Context, string, *backendclient.AttestationRequest, string) error {
	return nil
}
func (f *fakeClient) RefreshToken(context.Context, string) (string, error) { return "", nil }
func (f *fakeClient) UpsertNode(_ context.Context, nodeUUID string, req *backendclient.NodeUpsertRequest, jwt string) error {
	f.nodeUUID = nodeUUID
	f.req = req
	f.jwt = jwt
	return nil
}

func TestBackendSinkExportNotReady(t *testing.T) {
	s := &backendSink{
		state:         fakeState{},
		clientFactory: backendclient.New,
	}

	err := s.Export(context.Background(), &inventory.Snapshot{})
	require.ErrorIs(t, err, inventory.ErrNotReady)
}

func TestBackendSinkExportErrors(t *testing.T) {
	err := (&backendSink{}).Export(context.Background(), &inventory.Snapshot{})
	require.ErrorContains(t, err, "agent state")

	err = (&backendSink{state: fakeState{baseURL: "https://example.com", jwt: "jwt"}}).Export(context.Background(), &inventory.Snapshot{})
	require.ErrorContains(t, err, "client factory")

	err = (&backendSink{
		state:         fakeState{err: errors.New("state error")},
		clientFactory: backendclient.New,
	}).Export(context.Background(), &inventory.Snapshot{})
	require.ErrorContains(t, err, "state error")

	err = (&backendSink{
		state:         fakeState{baseURL: "https://example.com", jwt: "jwt"},
		clientFactory: backendclient.New,
	}).Export(context.Background(), nil)
	require.ErrorContains(t, err, "inventory snapshot")

	err = (&backendSink{
		state: fakeState{baseURL: "https://example.com", jwt: "jwt", nodeUUID: "node-1"},
		clientFactory: func(string) (backendclient.Client, error) {
			return nil, errors.New("client factory error")
		},
	}).Export(context.Background(), &inventory.Snapshot{})
	require.ErrorContains(t, err, "create backend client")
}

func TestBackendSinkExportUsesState(t *testing.T) {
	client := &fakeClient{}
	s := &backendSink{
		state: fakeState{
			baseURL:  "https://example.com",
			jwt:      "jwt-token",
			nodeUUID: "node-1",
		},
		clientFactory: func(string) (backendclient.Client, error) {
			return client, nil
		},
	}

	err := s.Export(context.Background(), &inventory.Snapshot{
		Hostname:  "host-a",
		MachineID: "machine-id",
	})
	require.NoError(t, err)
	require.Equal(t, "node-1", client.nodeUUID)
	require.Equal(t, "jwt-token", client.jwt)
	require.NotNil(t, client.req)
	require.Equal(t, "host-a", client.req.Hostname)
}
