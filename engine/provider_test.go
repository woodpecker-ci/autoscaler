package engine

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"

	"go.woodpecker-ci.org/autoscaler/config"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

var testUserDataStr = `
image: {{ .Image }}
environment:
	{{- range $key, $value := .Environment }}
	- {{ $key }}={{ $value }}
	{{- end }}
`

var testUserDataTmpl = template.Must(template.New("test").Parse(testUserDataStr))

func TestRenderUserDataTemplate(t *testing.T) {
	config := &config.Config{
		Image:       "test-image",
		GRPCAddress: "test-address",
		GRPCSecure:  false,
		Environment: map[string]string{
			"FOO": "bar",
		},
	}
	agent := &woodpecker.Agent{
		Token: "test-token",
	}

	userData, err := RenderUserDataTemplate(config, agent, testUserDataTmpl)

	assert.NoError(t, err)
	assert.Contains(t, userData, "test-image")
	assert.Contains(t, userData, "bar")
	assert.Contains(t, userData, "WOODPECKER_SERVER=test-address")
	assert.Contains(t, userData, "WOODPECKER_AGENT_SECRET=test-token")
}

func TestRenderUserDataTemplate_Secure(t *testing.T) {
	config := &config.Config{
		GRPCSecure: true,
	}
	agent := &woodpecker.Agent{}

	userData, err := RenderUserDataTemplate(config, agent, testUserDataTmpl)

	assert.NoError(t, err)
	assert.Contains(t, userData, "WOODPECKER_GRPC_SECURE=true")
}

func TestRenderUserDataTemplate_Error(t *testing.T) {
	config := &config.Config{}
	agent := &woodpecker.Agent{}
	tmpl := template.Must(template.New("test").Parse("{{.Missing}}"))

	_, err := RenderUserDataTemplate(config, agent, tmpl)
	assert.Error(t, err)
}
