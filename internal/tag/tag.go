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

// Package tag owns parsing and validation of agent tags.
package tag

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	EnvNodeGroup   = "FLEETINT_NODEGROUP"
	EnvComputeZone = "FLEETINT_COMPUTE_ZONE"
	EnvCustomTags  = "FLEETINT_TAGS"

	ReservedKeyNodeGroup   = "nodegroup"
	ReservedKeyComputeZone = "compute_zone"
)

var (
	validTagKey = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

	// ReservedKeys are special tags controlled by dedicated policy.
	ReservedKeys = map[string]struct{}{
		ReservedKeyNodeGroup:   {},
		ReservedKeyComputeZone: {},
	}
)

func IsReservedKey(key string) bool {
	_, ok := ReservedKeys[normalizeKey(key)]
	return ok
}

func ParseFromEnv() (map[string]string, error) {
	tags := map[string]string{}
	if value := strings.TrimSpace(os.Getenv(EnvNodeGroup)); value != "" {
		tags[ReservedKeyNodeGroup] = value
	}
	if value := strings.TrimSpace(os.Getenv(EnvComputeZone)); value != "" {
		tags[ReservedKeyComputeZone] = value
	}

	rawCustom := strings.TrimSpace(os.Getenv(EnvCustomTags))
	if rawCustom == "" {
		if err := ValidateReservedPairPatch(tags); err != nil {
			return nil, err
		}
		return tags, nil
	}
	custom, err := ParseCommaSeparatedPairs(rawCustom)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", EnvCustomTags, err)
	}
	for key := range custom {
		if IsReservedKey(key) {
			return nil, fmt.Errorf("%s does not accept reserved key %q", EnvCustomTags, key)
		}
	}
	for key, value := range custom {
		tags[key] = value
	}
	if err := ValidateReservedPairPatch(tags); err != nil {
		return nil, err
	}
	return tags, nil
}

func ParseCommaSeparatedPairs(raw string) (map[string]string, error) {
	result := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, err := parseSinglePair(part)
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func ParseCLIArgs(args []string) (map[string]string, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("at least one tag assignment is required")
	}
	result := map[string]string{}
	for _, arg := range args {
		key, value, err := parseSinglePair(stripPrefix(arg))
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", arg, err)
		}
		result[key] = value
	}
	if err := ValidateReservedPairPatch(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ValidateReservedPairPatch validates reserved key combinations in a patch request.
// compute_zone may only be changed when nodegroup is included in the same patch.
func ValidateReservedPairPatch(tags map[string]string) error {
	nodeGroupValue, hasNodeGroup := tags[ReservedKeyNodeGroup]
	computeZoneValue, hasComputeZone := tags[ReservedKeyComputeZone]
	nodeGroupValue = strings.TrimSpace(nodeGroupValue)
	computeZoneValue = strings.TrimSpace(computeZoneValue)

	if hasComputeZone && !hasNodeGroup {
		return fmt.Errorf("%q can only be updated when %q is also provided", ReservedKeyComputeZone, ReservedKeyNodeGroup)
	}
	// Non-empty updates for either reserved key require both keys to be present and non-empty.
	if hasNodeGroup && nodeGroupValue != "" && (!hasComputeZone || computeZoneValue == "") {
		return fmt.Errorf("%q requires a non-empty %q in the same update", ReservedKeyNodeGroup, ReservedKeyComputeZone)
	}
	if hasComputeZone && computeZoneValue != "" && (!hasNodeGroup || nodeGroupValue == "") {
		return fmt.Errorf("%q requires a non-empty %q in the same update", ReservedKeyComputeZone, ReservedKeyNodeGroup)
	}
	return nil
}

func MergeMissing(base, incoming map[string]string) map[string]string {
	out := Clone(base)
	for key, value := range incoming {
		if _, ok := out[key]; ok {
			continue
		}
		out[key] = value
	}
	return out
}

func Clone(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		out[key] = value
	}
	return out
}

func NormalizeAndValidateKey(raw string) (string, error) {
	key := normalizeKey(raw)
	if key == "" {
		return "", fmt.Errorf("tag key is empty")
	}
	if !validTagKey.MatchString(key) {
		return "", fmt.Errorf("invalid tag key %q", key)
	}
	return key, nil
}

func parseSinglePair(raw string) (key, value string, err error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected key=value")
	}

	key, err = NormalizeAndValidateKey(parts[0])
	if err != nil {
		return "", "", err
	}

	value = strings.TrimSpace(parts[1])
	if value == "" {
		return "", "", fmt.Errorf("tag value is empty")
	}

	return key, value, nil
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func stripPrefix(arg string) string {
	arg = strings.TrimSpace(arg)
	arg = strings.TrimPrefix(arg, "--")
	return strings.TrimPrefix(arg, "-")
}
