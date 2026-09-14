// Copyright 2026 The A2A Authors
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

package cliplugin

import (
	"encoding/json"
	"fmt"
	"os"
)

// ServeInfo writes the Info JSON document to stdout and exits the process
// with code 0. Call this when os.Args[1] == SubcommandInfo.
func ServeInfo(info Info) {
	if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
		fmt.Fprintf(os.Stderr, "cliplugin: writing info: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
