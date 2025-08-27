// SPDX-FileCopyrightText: Copyright (c) 2025, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

package main

// @title GPUhealth API
// @version 1.0
// @description GPU monitoring and management daemon API
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:15133
// @BasePath /

import (
	"fmt"
	"os"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/cmd/gpuhealth/command"
)

func main() {
	app := command.App()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}
}
