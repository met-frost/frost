// Package dataset implements the general I/O dataset for the /obs/* routes.
package dataset

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/xeipuuv/gojsonschema"
	"gitlab.met.no/frost/frost/internal/common"
)

func datasetSchema() string {
	return `{
		"title": "dataset",
		"type": "object",
		"properties": {
		    "tstype": {
				"type": "string"
			},
		    "tseries": {
			    "type": "array",
			    "items": {
			        "type": "object",
			        "properties": {
				        "header": {
				            "type": "object",
				            "properties": {
					            "id": {
									"type": "object"
								},
					            "extra": {
									"oneOf": [
										{
											"type": "null"
										},
										{
											"type": "object"
										}
									]
								}
				            },
							"required": ["id"],
				            "additionalProperties": false
				        },
				        "observations": {
							"type": "array",
							"items": {
				                "type": "object",
				                "properties": {
					                "time": {
										"type": "string"
									},
					                "body": {
									    "oneOf": [
											{
												"type": "null"
											},
											{
												"type": "object"
											}
										]
								    }
				                },
				                "required": ["time", "body"],
							    "additionalProperties": false
						    }
				        }
			        },
			        "required": ["header"],
			        "additionalProperties": false
			    }
		    }
		},
		"required": ["tstype", "tseries"],
		"additionalProperties": false
	}`
}

// HdrIDPlaceholder returns the placeholder used for replacing with the hdr/id schema specific
// to the ts type.
func HdrIDPlaceholder() string {
	return "%hdr/id%"
}

// HdrExtraPlaceholder returns the placeholder used for replacing with the hdr/extra schema
// specific to the ts type.
func HdrExtraPlaceholder() string {
	return "%hdr/extra%"
}

// ObsBodyPlaceholder returns the placeholder used for replacing with the obs/body schema specific
// to the ts type.
func ObsBodyPlaceholder() string {
	return "%obs/body%"
}

// DatasetSchemaTemplateTsANY returns the variant of the dataset schema used for generating parts
// of the OpenAPI specification for a .../ts/{create|delete|update} endpoint. It contains
// placeholders to replace with tstype-specific sub-schemas for hdr/id and hdr/extra.
func DatasetSchemaTemplateTsANY() string {
	return fmt.Sprintf(`{
		"title": "dataset",
		"type": "object",
		"properties": {
		    "tstype": {
				"type": "string"
			},
		    "tseries": {
			    "type": "array",
			    "items": {
			        "type": "object",
			        "properties": {
				        "header": {
				            "type": "object",
				            "properties": {
					            "id": %s,
					            "extra": %s
				            },
							"required": ["id"],
				            "additionalProperties": false
				        },
				        "observations": null
			        },
			        "required": ["header", "observations"],
			        "additionalProperties": false
			    }
		    }
		},
		"required": ["tstype", "tseries"],
		"additionalProperties": false
	}`, HdrIDPlaceholder(), HdrExtraPlaceholder())
}

// DatasetSchemaTemplatePutGet returns the variant of the dataset schema used for generating parts
// of the OpenAPI specification for a .../put or .../get endpoint. It contains placeholders to
// replace with tstype-specific sub-schemas for hdr/id, hdr/extra, and obs/body.
func DatasetSchemaTemplatePutGet() string {
	return fmt.Sprintf(`{
		"title": "dataset",
		"type": "object",
		"properties": {
		    "tstype": {
				"type": "string"
			},
		    "tseries": {
			    "type": "array",
			    "items": {
			        "type": "object",
			        "properties": {
				        "header": {
				            "type": "object",
				            "properties": {
					            "id": %s,
					            "extra": %s
				            },
							"required": ["id"],
				            "additionalProperties": false
				        },
				        "observations": {
							"type": "array",
							"items": {
				                "type": "object",
				                "properties": {
					                "time": {
										"type": "string",
										"description": "Observation time. An <a
										href=\"https://en.wikipedia.org/wiki/ISO_8601\">
										ISO 8601</a> UTC time of the form
										'YYYY-MM-DDThh:mm:ssZ'. Seconds is currently the finest
										resolution supported.",
										"example": "2024-10-08T14:08:59Z"
									},
					                "body": %s
				                },
				                "required": ["time", "body"],
							    "additionalProperties": false
						    }
				        }
			        },
			        "required": ["header", "observations"],
			        "additionalProperties": false
			    }
		    }
		},
		"required": ["tstype", "tseries"],
		"additionalProperties": false
	}`, HdrIDPlaceholder(), HdrExtraPlaceholder(), ObsBodyPlaceholder())
}

var schemaLoader gojsonschema.JSONLoader

func init() {
	schemaLoader = gojsonschema.NewStringLoader(datasetSchema())
}

// Header represents a time series header.
type Header struct {
	ID        map[string]any `json:"id,omitempty"`        // defined by TimeSeries impl.
	Extra     map[string]any `json:"extra,omitempty"`     // --- '' ---
	Available map[string]any `json:"available,omitempty"` // defined separately
}

// Observation represents an observation.
type Observation struct {
	Time *time.Time     `json:"time,omitempty"`
	Body map[string]any `json:"body"` // defined by TimeSeries implementation
}

// SingleTSeries represents a time series of observation values.
type SingleTSeries struct {
	Header       Header        `json:"header"`
	Observations []Observation `json:"observations"`
}

// Dataset represents a set of time series of observations for a time series type.
type Dataset struct {
	TSeriesType string          `json:"tstype"`
	TSeries     []SingleTSeries `json:"tseries"`
}

// Validate validates dset against the schema. Returns nil if valid, otherwise an error.
func Validate(dset []byte) error {
	var m map[string]any
	err := json.Unmarshal(dset, &m)
	if err != nil {
		return fmt.Errorf("json.Unmarshal() failed: %v", err)
	}
	return common.SchemaValidate(schemaLoader, m)
}

// TotalObsCount returns the total number of observations in a dataset.
func (dset Dataset) TotalObsCount() int {
	n := 0
	for _, sts := range dset.TSeries {
		n += len(sts.Observations)
	}
	return n
}

// Pos represents a geographical position.
type Pos struct {
	Lon   float64 `json:"lon,string"`
	Lat   float64 `json:"lat,string"`
	Valid bool    // true iff individual observation has an explicit position
}

// UnmarshalJSON is a custom implementation to ensure that Pos.valid is set
// to true iff the parsing was successful.
func (p *Pos) UnmarshalJSON(b []byte) error {
	var x struct {
		Lon float64 `json:"lon,string"`
		Lat float64 `json:"lat,string"`
	}

	if err := json.Unmarshal(b, &x); err != nil {
		*p = Pos{Valid: false}
		return fmt.Errorf("failed to parse pos: %v", err)
	}

	if string(b) == "null" { // JSON null value?
		*p = Pos{Valid: false}
	} else {
		*p = Pos{x.Lon, x.Lat, true}
	}

	return nil
}

// MarshalJSON is a custom implementation to ensure to marshall into a JSON string with Lon and Lat
// if Pos.valid is true and otherwise into a null value.
func (p Pos) MarshalJSON() ([]byte, error) {
	type x struct {
		Lon float64 `json:"lon,string"`
		Lat float64 `json:"lat,string"`
	}

	if p.Valid {
		return json.Marshal(x{p.Lon, p.Lat})
	}
	return json.Marshal(nil)
}

// MakePos creates and initializes an new valid instance of Pos (i.e. a position
// for an individual observation with an explicit position).
func MakePos(lon, lat float64) Pos {
	return Pos{lon, lat, true}
}

// MakeNullPos creates and initializes an new invalid instance of Pos (i.e. a position
// for an observation without an explicit position).
func MakeNullPos() Pos {
	return Pos{Valid: false}
}
