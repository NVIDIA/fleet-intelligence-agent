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

package attestationloop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
)

type stubState struct {
	baseURL string
	baseOK  bool
	baseErr error
	jwt     string
	jwtOK   bool
	jwtErr  error
	setJWT  string
	nodeID  string
	nodeOK  bool
	nodeErr error
}

func (s *stubState) GetBackendBaseURL(context.Context) (string, bool, error) {
	return s.baseURL, s.baseOK, s.baseErr
}
func (s *stubState) SetBackendBaseURL(context.Context, string) error { return nil }
func (s *stubState) GetJWT(context.Context) (string, bool, error)    { return s.jwt, s.jwtOK, s.jwtErr }
func (s *stubState) SetJWT(_ context.Context, v string) error        { s.setJWT = v; s.jwt = v; return nil }
func (s *stubState) GetSAK(context.Context) (string, bool, error)    { return "", false, nil }
func (s *stubState) SetSAK(context.Context, string) error            { return nil }
func (s *stubState) GetNodeID(context.Context) (string, bool, error) {
	return s.nodeID, s.nodeOK, s.nodeErr
}
func (s *stubState) SetNodeID(context.Context, string) error { return nil }

type recordingClient struct {
	lastNodeID string
	lastJWT    string
	lastReq    *backendclient.AttestationRequest
	nonceResp  *backendclient.NonceResponse
}

func (c *recordingClient) Enroll(context.Context, string) (string, error) { return "", nil }
func (c *recordingClient) UpsertNode(context.Context, string, *backendclient.NodeUpsertRequest, string) error {
	return nil
}
func (c *recordingClient) GetNonce(context.Context, string, string) (*backendclient.NonceResponse, error) {
	return c.nonceResp, nil
}
func (c *recordingClient) SubmitAttestation(_ context.Context, nodeID string, req *backendclient.AttestationRequest, jwt string) error {
	c.lastNodeID = nodeID
	c.lastJWT = jwt
	c.lastReq = req
	return nil
}
func (c *recordingClient) RefreshToken(context.Context, string) (string, error) { return "", nil }

func TestToAttestationRequest(t *testing.T) {
	refreshTS := time.Now().UTC()
	req := toAttestationRequest(&Result{
		NonceRefreshTimestamp: refreshTS,
		Success:               true,
		ErrorMessage:          "",
		SDKResponse: SDKResponse{
			ResultCode:    7,
			ResultMessage: "ok",
			Evidences: []EvidenceItem{{
				Arch:          "BLACKWELL",
				Certificate:   "cert",
				DriverVersion: "575.1",
				Evidence:      "blob",
				Nonce:         "nonce",
				VBIOSVersion:  "vbios",
				Version:       "1.0",
			}},
		},
	})
	require.NotNil(t, req)
	require.Equal(t, refreshTS, req.AttestationData.NonceRefreshTimestamp)
	require.True(t, req.AttestationData.Success)
	require.Len(t, req.AttestationData.SDKResponse.Evidences, 1)
	require.Equal(t, "BLACKWELL", req.AttestationData.SDKResponse.Evidences[0].Arch)
	require.Nil(t, toAttestationRequest(nil))
}

func TestStateBackendClientFactory(t *testing.T) {
	orig := newBackendClient
	t.Cleanup(func() { newBackendClient = orig })

	client := &recordingClient{}
	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		require.Equal(t, "https://backend.example.com", rawBaseURL)
		return client, nil
	}

	factory := &stateBackendClientFactory{state: &stubState{baseURL: "https://backend.example.com", baseOK: true}}
	got, err := factory.client(context.Background())
	require.NoError(t, err)
	require.Equal(t, client, got)

	_, err = (&stateBackendClientFactory{}).client(context.Background())
	require.ErrorContains(t, err, "requires agent state")

	_, err = (&stateBackendClientFactory{state: &stubState{baseErr: errors.New("boom")}}).client(context.Background())
	require.ErrorContains(t, err, "boom")

	_, err = (&stateBackendClientFactory{state: &stubState{baseOK: false}}).client(context.Background())
	require.ErrorContains(t, err, "backend base URL not available")
}

func TestStateProvidersAndSubmitter(t *testing.T) {
	orig := newBackendClient
	t.Cleanup(func() { newBackendClient = orig })

	recording := &recordingClient{
		nonceResp: &backendclient.NonceResponse{
			Nonce:                 "abc123",
			NonceRefreshTimestamp: time.Unix(10, 0).UTC(),
			JWTAssertion:          "new-jwt",
		},
	}
	newBackendClient = func(rawBaseURL string) (backendclient.Client, error) {
		require.Equal(t, "https://backend.example.com", rawBaseURL)
		return recording, nil
	}

	state := &stubState{
		baseURL: "https://backend.example.com", baseOK: true,
		jwt: "jwt-token", jwtOK: true,
		nodeID: "node-1", nodeOK: true,
	}

	jwtProvider := NewStateJWTProvider(state)
	jwt, err := jwtProvider.GetJWT(context.Background())
	require.NoError(t, err)
	require.Equal(t, "jwt-token", jwt)
	require.NoError(t, jwtProvider.SetJWT(context.Background(), "updated"))
	require.Equal(t, "updated", state.setJWT)

	nodeID, err := NewStateNodeIDProvider(state)(context.Background())
	require.NoError(t, err)
	require.Equal(t, "node-1", nodeID)

	nonce, ts, refreshedJWT, err := NewStateNonceProvider(state).GetNonce(context.Background(), "node-1", "jwt-token")
	require.NoError(t, err)
	require.Equal(t, "abc123", nonce)
	require.Equal(t, time.Unix(10, 0).UTC(), ts)
	require.Equal(t, "new-jwt", refreshedJWT)

	result := &Result{
		NodeID: "node-1",
		SDKResponse: SDKResponse{
			ResultCode: 1,
			Evidences:  []EvidenceItem{{Arch: "BLACKWELL"}},
		},
	}
	err = NewStateBackendSubmitter(state).Submit(context.Background(), result, "jwt-token")
	require.NoError(t, err)
	require.Equal(t, "node-1", recording.lastNodeID)
	require.Equal(t, "jwt-token", recording.lastJWT)
	require.NotNil(t, recording.lastReq)
	require.Equal(t, "BLACKWELL", recording.lastReq.AttestationData.SDKResponse.Evidences[0].Arch)
}

func TestLegacyAttestationData(t *testing.T) {
	result := &Result{
		NonceRefreshTimestamp: time.Unix(20, 0).UTC(),
		Success:               false,
		ErrorMessage:          "boom",
		SDKResponse: SDKResponse{
			ResultCode:    9,
			ResultMessage: "bad",
			Evidences:     []EvidenceItem{{Arch: "BLACKWELL"}},
		},
	}
	legacy := result.LegacyAttestationData()
	require.NotNil(t, legacy)
	require.False(t, legacy.Success)
	require.Equal(t, "boom", legacy.ErrorMessage)
	require.Equal(t, 9, legacy.SDKResponse.ResultCode)
	require.Len(t, legacy.SDKResponse.Evidences, 1)
	require.Nil(t, (*Result)(nil).LegacyAttestationData())
}
