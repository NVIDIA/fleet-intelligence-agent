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
)

type testJWTProvider struct {
	jwt    string
	setJWT string
}

func (p *testJWTProvider) GetJWT(context.Context) (string, error) { return p.jwt, nil }
func (p *testJWTProvider) SetJWT(_ context.Context, value string) error {
	p.setJWT = value
	p.jwt = value
	return nil
}

type testNonceProvider struct {
	nonce        string
	refreshTS    time.Time
	refreshedJWT string
	err          error
}

func (p *testNonceProvider) GetNonce(context.Context, string, string) (string, time.Time, string, error) {
	return p.nonce, p.refreshTS, p.refreshedJWT, p.err
}

type testEvidenceCollector struct {
	resp *SDKResponse
	err  error
}

func (c *testEvidenceCollector) Collect(context.Context, string) (*SDKResponse, error) {
	return c.resp, c.err
}

type submitted struct {
	result *Result
	jwt    string
}

type testSubmitter struct {
	submitted submitted
	err       error
}

func (s *testSubmitter) Submit(_ context.Context, result *Result, jwt string) error {
	s.submitted = submitted{result: result, jwt: jwt}
	return s.err
}

func TestCollectOnceSuccess(t *testing.T) {
	refreshTS := time.Now().UTC()
	jwtProvider := &testJWTProvider{jwt: "old-jwt"}
	submitter := &testSubmitter{}
	manager := NewManager(
		func(context.Context) (string, error) { return "node-1", nil },
		jwtProvider,
		&testNonceProvider{nonce: "abc123", refreshTS: refreshTS, refreshedJWT: "new-jwt"},
		&testEvidenceCollector{resp: &SDKResponse{ResultCode: 200, ResultMessage: "ok"}},
		submitter,
		0,
	)

	result, err := manager.CollectOnce(context.Background())
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "node-1", result.NodeID)
	require.Equal(t, refreshTS, result.NonceRefreshTimestamp)
	require.Equal(t, "new-jwt", jwtProvider.setJWT)
	require.NotNil(t, submitter.submitted.result)
	require.Equal(t, "new-jwt", submitter.submitted.jwt)
	require.True(t, submitter.submitted.result.Success)
}

func TestCollectOnceCollectorFailureStillSubmitsFailureResult(t *testing.T) {
	submitter := &testSubmitter{}
	manager := NewManager(
		func(context.Context) (string, error) { return "node-1", nil },
		&testJWTProvider{jwt: "jwt-token"},
		&testNonceProvider{nonce: "abc123"},
		&testEvidenceCollector{err: errors.New("collect failed")},
		submitter,
		0,
	)

	result, err := manager.CollectOnce(context.Background())
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "collect failed", result.ErrorMessage)
	require.NotNil(t, submitter.submitted.result)
	require.False(t, submitter.submitted.result.Success)
}

func TestCollectOnceMissingDependencies(t *testing.T) {
	_, err := NewManager(nil, nil, nil, nil, nil, 0).CollectOnce(context.Background())
	require.Error(t, err)
}

func TestManagerRunAndCachedResult(t *testing.T) {
	submitter := &testSubmitter{}
	mgr := NewManager(
		func(context.Context) (string, error) { return "node-1", nil },
		&testJWTProvider{jwt: "jwt-token"},
		&testNonceProvider{nonce: "abc123"},
		&testEvidenceCollector{resp: &SDKResponse{ResultCode: 200}},
		submitter,
		5*time.Millisecond,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.NoError(t, mgr.Run(ctx))

	last := mgr.LastResult()
	require.NotNil(t, last)
	require.Equal(t, "node-1", last.NodeID)
	require.True(t, mgr.IsResultUpdated(time.Time{}))
}

func TestManagerHelpersAndSubmitterErrors(t *testing.T) {
	mgr := NewManager(
		func(context.Context) (string, error) { return "node-1", nil },
		&testJWTProvider{jwt: "jwt-token"},
		&testNonceProvider{nonce: "abc123"},
		&testEvidenceCollector{resp: &SDKResponse{}},
		&testSubmitter{},
		0,
	)
	require.Nil(t, mgr.LastResult())
	require.False(t, mgr.IsResultUpdated(time.Now().UTC()))

	err := NewBackendSubmitter(nil).Submit(context.Background(), &Result{}, "jwt")
	require.ErrorContains(t, err, "backend client")
	err = NewBackendSubmitter(&recordingClient{}).Submit(context.Background(), nil, "jwt")
	require.ErrorContains(t, err, "requires result")
	err = NewBackendSubmitter(&recordingClient{}).Submit(context.Background(), &Result{}, "")
	require.ErrorContains(t, err, "requires jwt")
}

func TestStateJWTProviderAndNodeIDProviderErrors(t *testing.T) {
	_, err := NewStateJWTProvider(nil).GetJWT(context.Background())
	require.ErrorContains(t, err, "requires agent state")
	err = NewStateJWTProvider(nil).SetJWT(context.Background(), "x")
	require.ErrorContains(t, err, "requires agent state")

	_, err = NewStateJWTProvider(&stubState{}).GetJWT(context.Background())
	require.ErrorContains(t, err, "jwt not available")

	_, err = NewStateNodeIDProvider(nil)(context.Background())
	require.ErrorContains(t, err, "requires agent state")
	_, err = NewStateNodeIDProvider(&stubState{})(context.Background())
	require.ErrorContains(t, err, "node ID not available")
}
