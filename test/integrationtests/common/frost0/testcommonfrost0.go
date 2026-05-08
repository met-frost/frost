package testcommonfrost0

import (
	"fmt"
	"time"

	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
)

// CreateObservation creates, from a set of components, a dataset.Observation with a frost0 body.
func CreateObservation(
	year int, month int, day int, hour int, minute int, second int, value int) dataset.Observation {
	tm := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return dataset.Observation{
		Time: &tm,
		Body: map[string]interface{}{
			"pos":     dataset.MakeNullPos(), // for now
			"value":   fmt.Sprintf("%d", value),
			"quality": "", // for now
		},
	}
}
