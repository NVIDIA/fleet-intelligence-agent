# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


# docker buildx build --platform linux/amd64,linux/arm64 -t nvcr.io/nvidian/gpu-health/gpuhealth-skyhook:0.0.1 --push .

FROM busybox:latest

RUN mkdir -p /skyhook-package/skyhook_dir

# Copy Skyhook package payload
COPY config.json /skyhook-package/config.json
COPY skyhook_dir/ /skyhook-package/skyhook_dir/
