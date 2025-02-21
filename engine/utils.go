package engine

import "go.woodpecker-ci.org/woodpecker/v3/woodpecker-go/woodpecker"

func countTasksByLabel(jobs []woodpecker.Task, labelKey, labelValue string) int {
	count := 0
	for _, job := range jobs {
		val, exists := job.Labels[labelKey]
		if exists && val == labelValue {
			count++
		}
	}
	return count
}
