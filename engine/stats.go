package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Stats struct {
	WorkerCount        int `json:"worker_count"`
	PendingCount       int `json:"pending_count"`
	WaitingOnDepsCount int `json:"waiting_on_deps_count"`
	RunningCount       int `json:"running_count"`
	CompletedCount     int `json:"completed_count"`
}

type JobInformation struct {
	ID           string            `json:"id"`
	Data         string            `json:"data"`
	Labels       map[string]string `json:"labels"`
	Dependencies string            `json:"dependencies,omitempty"`
	RunOn        string            `json:"run_on"`
	DepStatus    string            `json:"-"` // don't need those
	AgentID      int               `json:"agent_id"`
}

type QueueInfo struct {
	Pending       []JobInformation `json:"pending,omitempty"`
	WaitingOnDeps string           `json:"-"` // don't need those
	Running       []JobInformation `json:"running,omitempty"`
	Stats         Stats            `json:"stats"`
	Paused        bool             `json:"paused"`
}

// TODO: implement this into the official woodpecker-go client
func (a *Autoscaler) _getQueueInfo(target interface{}) error {
	apiRoute := fmt.Sprintf("%s/api/queue/info", a.config.APIUrl)
	req, err := http.NewRequest(http.MethodGet, apiRoute, nil)
	if err != nil {
		return fmt.Errorf("could not create queue request: %s", err.Error())
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.config.APIToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not query queue info: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Error from queue info api: %s", err.Error())
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (a *Autoscaler) getQueueInfo(_ context.Context) (freeTasks, runningTasks, pendingTasks int, err error) {
	queueInfo := new(QueueInfo)
	err = a._getQueueInfo(queueInfo)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("Error from QueueInfo: %s", err.Error())
	}

	if a.config.LabelsFilter == "" {
		return queueInfo.Stats.WorkerCount, queueInfo.Stats.RunningCount, queueInfo.Stats.PendingCount, nil
	}

	labelFilterKey, labelFilterValue, ok := strings.Cut(a.config.LabelsFilter, "=")
	if !ok {
		return 0, 0, 0, fmt.Errorf("Invalid labels filter: %s", a.config.LabelsFilter)
	}

	running := 0
	if queueInfo.Stats.RunningCount > 0 {
		if queueInfo.Running != nil {
			for _, runningJobs := range queueInfo.Running {
				val, exists := runningJobs.Labels[labelFilterKey]
				if exists && val == labelFilterValue {
					running++
				}
			}
		}
	}

	pending := 0
	if queueInfo.Stats.PendingCount > 0 {
		if queueInfo.Pending != nil {
			for _, pendingJobs := range queueInfo.Pending {
				val, exists := pendingJobs.Labels[labelFilterKey]
				if exists && val == labelFilterValue {
					pending++
				}
			}
		}
	}

	return queueInfo.Stats.WorkerCount, running, pending, nil
}
