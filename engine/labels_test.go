package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.woodpecker-ci.org/autoscaler/engine/types"
)

func Test_agentLabelsFor(t *testing.T) {
	t.Run("populates platform and backend from capability", func(t *testing.T) {
		got := agentLabelsFor(types.Capability{
			Platform: "linux/amd64",
			Backend:  types.BackendDocker,
		}, nil)

		assert.Equal(t, "linux/amd64", got.Labels["platform"])
		assert.Equal(t, "docker", got.Labels["backend"])
		assert.Empty(t, got.Mandatory)
	})

	t.Run("merges extra labels on top", func(t *testing.T) {
		got := agentLabelsFor(types.Capability{
			Platform: "linux/arm64",
			Backend:  types.BackendKubernetes,
		}, map[string]string{
			"location": "europe",
			"weather":  "sun",
		})

		assert.Equal(t, "linux/arm64", got.Labels["platform"])
		assert.Equal(t, "kubernetes", got.Labels["backend"])
		assert.Equal(t, "europe", got.Labels["location"])
		assert.Equal(t, "sun", got.Labels["weather"])
		assert.Empty(t, got.Mandatory)
	})

	t.Run("strips ! prefix and tracks mandatory keys", func(t *testing.T) {
		got := agentLabelsFor(types.Capability{
			Platform: "linux/amd64",
			Backend:  types.BackendDocker,
		}, map[string]string{
			"!special": "yes",
			"region":   "eu",
		})

		assert.Equal(t, "yes", got.Labels["special"])
		assert.Equal(t, "eu", got.Labels["region"])
		_, isMandatory := got.Mandatory["special"]
		assert.True(t, isMandatory, "expected 'special' to be mandatory")
		_, regionMandatory := got.Mandatory["region"]
		assert.False(t, regionMandatory, "expected 'region' to be non-mandatory")
	})

	t.Run("extra labels override capability values", func(t *testing.T) {
		// If an operator explicitly sets `platform=*` in extra labels, that
		// wins over the implicit capability value. This matches the docs:
		// "By default, agents provide ... labels which can be overwritten".
		got := agentLabelsFor(types.Capability{
			Platform: "linux/amd64",
			Backend:  types.BackendDocker,
		}, map[string]string{
			"platform": "*",
		})

		assert.Equal(t, "*", got.Labels["platform"])
	})
}

func Test_taskMatchesAgent(t *testing.T) {
	docker := agentLabelsFor(types.Capability{
		Platform: "linux/amd64",
		Backend:  types.BackendDocker,
	}, nil)

	t.Run("empty task labels match any agent", func(t *testing.T) {
		assert.True(t, taskMatchesAgent(nil, docker))
		assert.True(t, taskMatchesAgent(map[string]string{}, docker))
	})

	t.Run("matches when every task label is satisfied", func(t *testing.T) {
		assert.True(t, taskMatchesAgent(map[string]string{
			"platform": "linux/amd64",
			"backend":  "docker",
		}, docker))
	})

	t.Run("rejects mismatched value", func(t *testing.T) {
		assert.False(t, taskMatchesAgent(map[string]string{
			"platform": "linux/arm64",
		}, docker))
	})

	t.Run("rejects unknown key on agent", func(t *testing.T) {
		assert.False(t, taskMatchesAgent(map[string]string{
			"location": "europe",
		}, docker))
	})

	t.Run("ignores empty workflow label values", func(t *testing.T) {
		assert.True(t, taskMatchesAgent(map[string]string{
			"platform": "linux/amd64",
			"hostname": "", // empty per docs is ignored
		}, docker))
	})

	t.Run("agent wildcard matches any value", func(t *testing.T) {
		agent := agentLabelsFor(types.Capability{
			Platform: "linux/amd64",
			Backend:  types.BackendDocker,
		}, map[string]string{
			"region": "*",
		})

		assert.True(t, taskMatchesAgent(map[string]string{
			"region": "anywhere",
		}, agent))
	})

	t.Run("mandatory key requires explicit task label", func(t *testing.T) {
		agent := agentLabelsFor(types.Capability{
			Platform: "linux/amd64",
			Backend:  types.BackendDocker,
		}, map[string]string{
			"!gpu": "true",
		})

		// Task does not mention "gpu" -> mandatory key not satisfied.
		assert.False(t, taskMatchesAgent(map[string]string{
			"platform": "linux/amd64",
		}, agent))

		// Empty value also doesn't count.
		assert.False(t, taskMatchesAgent(map[string]string{
			"gpu": "",
		}, agent))

		// Wrong value -> rejected by the value check.
		assert.False(t, taskMatchesAgent(map[string]string{
			"gpu": "false",
		}, agent))

		// Matching value is accepted.
		assert.True(t, taskMatchesAgent(map[string]string{
			"gpu": "true",
		}, agent))
	})
}
