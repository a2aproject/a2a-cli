package testutil

import (
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func AllArtifactText(task *a2a.Task) string {
	var sb strings.Builder
	for _, art := range task.Artifacts {
		for _, p := range art.Parts {
			sb.WriteString(p.Text())
		}
	}
	return sb.String()
}
