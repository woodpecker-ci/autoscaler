package engine

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

type Provider interface {
	DeployAgent(context.Context, *woodpecker.Agent) error
	RemoveAgent(context.Context, *woodpecker.Agent) error
	ListDeployedAgentNames(context.Context) ([]string, error)
}

// RenderUserDataTemplate renders the user data template for an Agent
// using the provided configuration.
func RenderUserDataTemplate(config *config.Config, agent *woodpecker.Agent, tmpl *template.Template) (string, error) {
	params := struct {
		Image       string
		Environment map[string]string
	}{
		Image: config.Image,
		Environment: map[string]string{
			"WOODPECKER_SERVER":        config.GRPCAddress,
			"WOODPECKER_AGENT_SECRET":  agent.Token,
			"WOODPECKER_MAX_WORKFLOWS": fmt.Sprintf("%d", config.WorkflowsPerAgent),
		},
	}

	if config.GRPCSecure {
		params.Environment["WOODPECKER_GRPC_SECURE"] = "true"
	}

	for key, value := range config.Environment {
		params.Environment[key] = value
	}

	var userData bytes.Buffer
	if err := tmpl.Execute(&userData, params); err != nil {
		return "", err
	}

	return userData.String(), nil
}
