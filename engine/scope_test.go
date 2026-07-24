package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.woodpecker-ci.org/autoscaler/engine/types"
	"go.woodpecker-ci.org/woodpecker/v3/pipeline"
	"go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"
)

// serverTaskLabels returns the label set a genuine task exposes off
// /queue/info for the given workflow-declared labels: the server stamps
// org-id/repo (ApplyLabelsFromRepo) and adds internal "woodpecker-ci.org/*"
// bookkeeping labels the scheduler strips before matching.
func serverTaskLabels(workflow map[string]string) map[string]string {
	labels := map[string]string{
		pipeline.LabelFilterOrg:  "123",
		pipeline.LabelFilterRepo: "acme/widgets",
		pipeline.LabelRepoID:     "42",
		pipeline.LabelBranch:     "main",
	}
	for key, value := range workflow {
		labels[key] = value
	}
	return labels
}

// instanceAdminBuckets renders buckets the way NewAutoscaler does for a global
// pool, so tests exercise the same scope-enriched labels as production.
func instanceAdminBuckets(capabilities ...types.Capability) []agentBucket {
	scope := instanceAdminPoolScope()
	buckets := make([]agentBucket, 0, len(capabilities))
	for _, capability := range capabilities {
		buckets = append(buckets, agentBucket{
			Capability: capability,
			Labels:     scope.agentLabelsFor(capability, nil),
		})
	}
	return buckets
}

func Test_poolScopeAgentLabels(t *testing.T) {
	scope := poolScope{
		enforcedLabels: map[string]string{pipeline.LabelFilterOrg: labelWildcard},
	}

	labels := scope.agentLabelsFor(dockerAmd64Cap, map[string]string{
		pipeline.LabelFilterOrg:                        "123",
		labelMandatoryPrefix + pipeline.LabelFilterOrg: "456",
	})

	assert.Equal(t, labelWildcard, labels.Labels[pipeline.LabelFilterOrg], "server-enforced scope wins over a normal custom label")
	assert.Equal(t, "456", labels.Mandatory[pipeline.LabelFilterOrg], "a mandatory custom label stays independent")
}

func Test_taskMatchesAgent_serverLabels(t *testing.T) {
	agent := instanceAdminPoolScope().agentLabelsFor(dockerAmd64Cap, nil)

	t.Run("matches a real task carrying org-id/repo/internal labels", func(t *testing.T) {
		assert.True(t, taskMatchesAgent(serverTaskLabels(map[string]string{
			pipeline.LabelFilterPlatform: "linux/amd64",
		}), agent))
	})

	t.Run("still rejects a real task for a different platform", func(t *testing.T) {
		assert.False(t, taskMatchesAgent(serverTaskLabels(map[string]string{
			pipeline.LabelFilterPlatform: "linux/arm64",
		}), agent))
	})

	t.Run("org-id/repo alone keep a task routable", func(t *testing.T) {
		assert.True(t, taskMatchesAgent(serverTaskLabels(nil), agent))
	})

	t.Run("internal woodpecker-ci.org labels are ignored", func(t *testing.T) {
		assert.True(t, taskMatchesAgent(map[string]string{
			pipeline.LabelRepoID:         "42",
			pipeline.LabelFilterPlatform: "linux/amd64",
		}, agent))
	})
}

func Test_taskMatchesAgent_mandatoryExactValue(t *testing.T) {
	t.Run("a mandatory value must match exactly", func(t *testing.T) {
		agent := agentLabelsFor(dockerAmd64Cap, map[string]string{"!gpu": "true"})
		assert.True(t, taskMatchesAgent(map[string]string{
			pipeline.LabelFilterPlatform: "linux/amd64",
			"gpu":                        "true",
		}, agent))
		assert.False(t, taskMatchesAgent(map[string]string{
			pipeline.LabelFilterPlatform: "linux/amd64",
			"gpu":                        "false",
		}, agent))
	})

	t.Run("a mandatory wildcard is not satisfied by an arbitrary value", func(t *testing.T) {
		// The server compares mandatory labels for an exact match, so "!gpu=*"
		// requires the literal value "*", which real tasks never carry.
		agent := agentLabelsFor(dockerAmd64Cap, map[string]string{"!gpu": labelWildcard})
		assert.False(t, taskMatchesAgent(map[string]string{
			pipeline.LabelFilterPlatform: "linux/amd64",
			"gpu":                        "true",
		}, agent))
	})
}

func Test_routeTaskToBucket_serverLabels(t *testing.T) {
	buckets := instanceAdminBuckets(dockerAmd64Cap, dockerArm64Cap)

	assert.Equal(t, 0, routeTaskToBucket(woodpecker.Task{
		Labels: serverTaskLabels(map[string]string{pipeline.LabelFilterPlatform: "linux/amd64"}),
	}, buckets))
	assert.Equal(t, 1, routeTaskToBucket(woodpecker.Task{
		Labels: serverTaskLabels(map[string]string{pipeline.LabelFilterPlatform: "linux/arm64"}),
	}, buckets))
}

func Test_computeBucketStates_serverLabels(t *testing.T) {
	buckets := instanceAdminBuckets(dockerAmd64Cap, dockerArm64Cap)
	pending := []woodpecker.Task{
		{Labels: serverTaskLabels(map[string]string{pipeline.LabelFilterPlatform: "linux/amd64"})},
		{Labels: serverTaskLabels(map[string]string{pipeline.LabelFilterPlatform: "linux/amd64"})},
		{Labels: serverTaskLabels(map[string]string{pipeline.LabelFilterPlatform: "linux/arm64"})},
	}

	states, unmatchedPending, _ := computeBucketStates(buckets, pending, nil, nil)

	assert.Zero(t, unmatchedPending, "real tasks must not be counted unschedulable")
	assert.Equal(t, 2, states[0].Pending)
	assert.Equal(t, 1, states[1].Pending)
}
