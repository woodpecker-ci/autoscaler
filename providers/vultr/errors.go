package vultr

import "errors"

var (
	ErrInvalidRegion = errors.New("no valid region set")
	ErrInvalidPlan   = errors.New("no valid plan set")
	ErrInvalidImage  = errors.New("no valid image set")
)
