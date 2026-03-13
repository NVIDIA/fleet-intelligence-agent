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

// Package precheck evaluates prerequisite checks for enrollment and installation flows.
package precheck

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	nvidianvml "github.com/NVIDIA/fleet-intelligence-sdk/pkg/nvidia-query/nvml"
	godcgm "github.com/NVIDIA/go-dcgm/pkg/dcgm"

	"github.com/NVIDIA/fleet-intelligence-agent/internal/machineinfo"
)

var supportedArchitectures = []string{"Hopper", "Blackwell", "Rubin"}

const minimumDCGMVersion = "4.2.3"

var (
	newNVML              = nvidianvml.New
	getMachineInfo       = machineinfo.GetMachineInfo
	lookPath             = exec.LookPath
	dcgmInit             = initDCGMStandalone
	getHostengineVersion = getDCGMHostengineVersion
	getenv               = os.Getenv
)

type nvmlInstance = nvidianvml.Instance

type Input struct {
	MachineInfo     *machineinfo.MachineInfo
	NVAttestPresent *bool
	DCGMReachable   *bool
	DCGMVersion     string
}

type Check struct {
	Name    string
	Passed  bool
	Message string
}

type Result struct {
	Checks []Check
}

func (r Result) Passed() bool {
	for _, check := range r.Checks {
		if !check.Passed {
			return false
		}
	}

	return true
}

func SupportedArchitectures() []string {
	return slices.Clone(supportedArchitectures)
}

func (r Result) FailedChecks() []Check {
	failed := make([]Check, 0, len(r.Checks))
	for _, check := range r.Checks {
		if !check.Passed {
			failed = append(failed, check)
		}
	}

	return failed
}

func Run() (Result, error) {
	input, err := CollectInput()
	if err != nil {
		return Result{}, err
	}

	return Evaluate(input), nil
}

func CollectInput() (Input, error) {
	input := Input{}

	nvmlInstance, err := newNVML()
	if err == nil {
		defer func() { _ = nvmlInstance.Shutdown() }()

		info, machineInfoErr := getMachineInfo(nvmlInstance)
		if machineInfoErr == nil {
			input.MachineInfo = info
		}
	}

	nvattestPresent := detectNVAttest()
	input.NVAttestPresent = &nvattestPresent

	dcgmReachable, dcgmVersion := detectDCGM()
	input.DCGMReachable = &dcgmReachable
	input.DCGMVersion = dcgmVersion

	return input, nil
}

func Evaluate(input Input) Result {
	checks := []Check{
		evaluateGPUPresence(input.MachineInfo),
		evaluateArchitecture(input.MachineInfo),
		evaluateDriver(input.MachineInfo),
		evaluateNVAttest(input.NVAttestPresent),
		evaluateDCGM(input),
	}

	return Result{Checks: checks}
}

func evaluateGPUPresence(info *machineinfo.MachineInfo) Check {
	if info == nil || info.GPUInfo == nil || len(info.GPUInfo.GPUs) == 0 {
		return Check{
			Name:    "gpu-present",
			Message: "no NVIDIA GPU detected",
		}
	}

	return Check{
		Name:    "gpu-present",
		Passed:  true,
		Message: "NVIDIA GPU detected",
	}
}

func evaluateArchitecture(info *machineinfo.MachineInfo) Check {
	if info == nil || info.GPUInfo == nil || len(info.GPUInfo.GPUs) == 0 {
		return Check{
			Name:    "gpu-architecture",
			Passed:  true,
			Message: "GPU architecture check skipped because no GPU was detected",
		}
	}

	if info.GPUInfo.Architecture == "" {
		return Check{
			Name:    "gpu-architecture",
			Message: "GPU architecture is unknown",
		}
	}

	if !slices.ContainsFunc(supportedArchitectures, func(s string) bool {
		return strings.EqualFold(s, info.GPUInfo.Architecture)
	}) {
		return Check{
			Name:    "gpu-architecture",
			Message: "unsupported GPU architecture: " + info.GPUInfo.Architecture,
		}
	}

	return Check{
		Name:    "gpu-architecture",
		Passed:  true,
		Message: "supported GPU architecture detected: " + info.GPUInfo.Architecture,
	}
}

func evaluateDriver(info *machineinfo.MachineInfo) Check {
	if info == nil || info.GPUInfo == nil || len(info.GPUInfo.GPUs) == 0 {
		return Check{
			Name:    "gpu-driver",
			Passed:  true,
			Message: "driver check skipped because no GPU was detected",
		}
	}

	if info.GPUDriverVersion == "" {
		return Check{
			Name:    "gpu-driver",
			Message: "NVIDIA driver not detected",
		}
	}

	return Check{
		Name:    "gpu-driver",
		Passed:  true,
		Message: "NVIDIA driver detected: " + info.GPUDriverVersion,
	}
}

// evaluateNVAttest checks whether nvattest is present in PATH.
// present may be nil when constructing a partial Input (e.g. in targeted tests
// that focus on other checks); in that case the check is skipped.
func evaluateNVAttest(present *bool) Check {
	if present == nil {
		return Check{
			Name:    "nvattest",
			Passed:  true,
			Message: "nvattest check skipped",
		}
	}

	if !*present {
		return Check{
			Name:    "nvattest",
			Message: "nvattest not found in PATH",
		}
	}

	return Check{
		Name:    "nvattest",
		Passed:  true,
		Message: "nvattest detected",
	}
}

func evaluateDCGM(input Input) Check {
	if input.DCGMReachable == nil {
		return Check{
			Name:    "dcgm",
			Passed:  true,
			Message: "DCGM checks skipped",
		}
	}

	if !*input.DCGMReachable {
		return Check{
			Name:    "dcgm",
			Message: "DCGM HostEngine is not reachable",
		}
	}

	if input.DCGMVersion == "" {
		return Check{
			Name:    "dcgm",
			Message: "DCGM HostEngine version could not be determined",
		}
	}

	versionOK, err := isVersionAtLeast(input.DCGMVersion, minimumDCGMVersion)
	if err != nil {
		return Check{
			Name:    "dcgm",
			Message: "failed to parse DCGM HostEngine version: " + err.Error(),
		}
	}

	if !versionOK {
		return Check{
			Name:    "dcgm",
			Message: "DCGM HostEngine version " + input.DCGMVersion + " is below required minimum " + minimumDCGMVersion,
		}
	}

	return Check{
		Name:    "dcgm",
		Passed:  true,
		Message: "DCGM HostEngine version is supported: " + input.DCGMVersion,
	}
}

func isVersionAtLeast(version, minimum string) (bool, error) {
	versionParts, err := parseVersion(version)
	if err != nil {
		return false, err
	}

	minimumParts, err := parseVersion(minimum)
	if err != nil {
		return false, err
	}

	for i := range min(len(versionParts), len(minimumParts)) {
		if versionParts[i] > minimumParts[i] {
			return true, nil
		}
		if versionParts[i] < minimumParts[i] {
			return false, nil
		}
	}

	return len(versionParts) >= len(minimumParts), nil
}

func parseVersion(version string) ([]int, error) {
	parts := strings.Split(version, ".")
	parsed := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid version %q", version)
		}
		parsed = append(parsed, value)
	}

	return parsed, nil
}

func detectNVAttest() bool {
	_, err := lookPath("nvattest")
	return err == nil
}

func detectDCGM() (bool, string) {
	cleanup, err := dcgmInit()
	if err != nil {
		return false, ""
	}
	defer cleanup()

	version, err := getHostengineVersion()
	if err != nil {
		return true, ""
	}

	return true, version
}

func extractVersion(raw string) string {
	for _, pair := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "version" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func initDCGMStandalone() (func(), error) {
	initParams := resolveDCGMInitFromEnv()
	return godcgm.Init(godcgm.Standalone, initParams.address, initParams.isUnixSocket)
}

func getDCGMHostengineVersion() (string, error) {
	versionInfo, err := godcgm.GetHostengineVersionInfo()
	if err != nil {
		return "", err
	}

	return extractVersion(versionInfo.RawBuildInfoString), nil
}

type dcgmInitParams struct {
	address      string
	isUnixSocket string
}

func resolveDCGMInitFromEnv() dcgmInitParams {
	address := strings.TrimSpace(getenv("DCGM_URL"))
	isUnixSocket := "0"

	if truthy, err := strconv.ParseBool(strings.TrimSpace(getenv("DCGM_URL_IS_UNIX_SOCKET"))); err == nil && truthy {
		isUnixSocket = "1"
	}

	if address == "" {
		address = "localhost"
	}

	return dcgmInitParams{
		address:      address,
		isUnixSocket: isUnixSocket,
	}
}
