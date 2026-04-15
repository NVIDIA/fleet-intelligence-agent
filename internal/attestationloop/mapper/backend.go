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

// Package mapper contains attestation loop payload mappers.
package mapper

import (
	"github.com/NVIDIA/fleet-intelligence-agent/internal/attestationloop"
	"github.com/NVIDIA/fleet-intelligence-agent/internal/backendclient"
)

// ToAttestationRequest maps an attestation result to the backend attestation contract.
func ToAttestationRequest(r attestationloop.Result) backendclient.AttestationRequest {
	req := backendclient.AttestationRequest{
		AttestationData: backendclient.AttestationData{
			NonceRefreshTimestamp: r.NonceRefreshTimestamp,
			Success:               r.Success,
			ErrorMessage:          r.ErrorMessage,
			SDKResponse: backendclient.AttestationSDKResponse{
				ResultCode:    r.SDKResponse.ResultCode,
				ResultMessage: r.SDKResponse.ResultMessage,
			},
		},
	}

	if len(r.SDKResponse.Evidences) > 0 {
		req.AttestationData.SDKResponse.Evidences = make([]backendclient.EvidenceItem, 0, len(r.SDKResponse.Evidences))
		for _, ev := range r.SDKResponse.Evidences {
			req.AttestationData.SDKResponse.Evidences = append(req.AttestationData.SDKResponse.Evidences, backendclient.EvidenceItem{
				Arch:          ev.Arch,
				Certificate:   ev.Certificate,
				DriverVersion: ev.DriverVersion,
				Evidence:      ev.Evidence,
				Nonce:         ev.Nonce,
				VBIOSVersion:  ev.VBIOSVersion,
				Version:       ev.Version,
			})
		}
	}

	return req
}
