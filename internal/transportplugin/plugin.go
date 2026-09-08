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

package transportplugin

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// IsBuiltin returns true if protocol is supported by the SDK natively.
func IsBuiltin(protocol a2a.TransportProtocol) bool {
	switch protocol {
	case a2a.TransportProtocolHTTPJSON, a2a.TransportProtocolJSONRPC, a2a.TransportProtocolGRPC:
		return true
	default:
		return false
	}
}

// Load loads a transport plugin as client factory option or returns
// an error if plugin is unavailable.
func Load(protocol a2a.TransportProtocol) (a2aclient.FactoryOption, error) {
	binary, err := findPlugin(string(protocol))
	if err != nil {
		return nil, err
	}
	return a2aclient.WithTransport(protocol, newPluginTransportFactory(binary, nil)), nil
}

// LoadForCard loads transport plugin for every custom transport listed in the card if
// a corresponding plugin can be found.
func LoadForCard(card *a2a.AgentCard) ([]a2aclient.FactoryOption, error) {
	seen := map[a2a.TransportProtocol]bool{}
	for _, iface := range card.SupportedInterfaces {
		seen[iface.ProtocolBinding] = true
	}
	var opts []a2aclient.FactoryOption
	for p := range seen {
		opt, err := Load(p)
		if err != nil {
			continue
		}
		opts = append(opts, opt)
	}

	return opts, nil
}

func findPlugin(name string) (string, error) {
	path, err := exec.LookPath(binaryName(name))
	if err != nil {
		var available []string
		for _, d := range discover() {
			available = append(available, d.Name)
		}
		availableStr := strings.Join(available, ", ")
		return "", fmt.Errorf("no transport plugin %q found on PATH (expected a %q binary); available plugins: [%s]", name, binaryName(name), availableStr)
	}
	return path, nil
}
