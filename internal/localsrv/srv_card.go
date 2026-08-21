// Copyright 2025 The A2A Authors
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

package localsrv

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// CardParams holds the parameters used to build a local agent card.
type CardParams struct {
	CardPath string

	AgentName        string
	AgentDesc        string
	AdvertiseAddress string
	Transport        a2a.TransportProtocol
}

func createAgentCard(cfg CardParams) (*a2a.AgentCard, error) {
	if cfg.CardPath != "" {
		data, err := os.ReadFile(cfg.CardPath)
		if err != nil {
			return nil, fmt.Errorf("reading card file: %w", err)
		}
		card := new(a2a.AgentCard)
		if err := json.Unmarshal(data, card); err != nil {
			return nil, fmt.Errorf("parsing card file: %w", err)
		}
		return card, nil
	}

	name := "a2a-cli"
	if cfg.AgentName != "" {
		name = cfg.AgentName
	}
	var url string
	if cfg.AdvertiseAddress != "" {
		url = cfg.AdvertiseAddress
	} else if cfg.Transport == a2a.TransportProtocolGRPC {
		url = "127.0.0.1"
	} else {
		url = "http://127.0.0.1"
	}
	return &a2a.AgentCard{
		Name:                name,
		Description:         cfg.AgentDesc,
		Version:             "1.0.0",
		Capabilities:        a2a.AgentCapabilities{Streaming: true},
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(url, cfg.Transport)},
		// defaultInputModes, defaultOutputModes and skills are REQUIRED in the proto.
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Skills:             []a2a.AgentSkill{},
	}, nil
}
