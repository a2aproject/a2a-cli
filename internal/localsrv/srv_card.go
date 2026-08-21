package localsrv

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

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
