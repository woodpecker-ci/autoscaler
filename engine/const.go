package engine

import "fmt"

var (
	LabelPrefix = "wp.autoscaler/"
	LabelPool   = fmt.Sprintf("%spool", LabelPrefix)
	LabelImage  = fmt.Sprintf("%simage", LabelPrefix)
)
