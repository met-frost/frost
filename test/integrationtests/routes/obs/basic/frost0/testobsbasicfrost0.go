package testobsbasicfrost0

import (
	"encoding/json"
	"fmt"
	"testing"

	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	dset "gitlab.met.no/frost/frost/internal/routes/api/insituobs/dataset"
	testcommonfrost0 "gitlab.met.no/frost/frost/test/integrationtests/common/frost0"
	rrt "gitlab.met.no/frost/frost/test/integrationtests/reqresptest"
)

// Creates tests for query parameter behavior wrt. wildcards and case sensitivity.
func createTestQueryParamWildcardAndCaseSensitivity() (rrt.GetTestSequence, error) {

	// --- STEP 1: create dataset ------

	// create dataset with four different element/source combos of a 'frost0' time series of
	// the same two observations
	observations := []dset.Observation{}
	observations = append(
		observations, testcommonfrost0.CreateObservation(2020, 1, 1, 0, 0, 0, 1000))
	observations = append(
		observations, testcommonfrost0.CreateObservation(2020, 1, 1, 0, 0, 1, 1001))

	tseries := []dset.SingleTSeries{}
	for _, combo := range []struct {
		element string
		source  string
	}{
		{"e0", "s0"},
		{"e0", "s1"},
		{"e1", "s0"},
		{"e1", "s1"},
	} {
		tseries = append(tseries, dset.SingleTSeries{
			Header: dataset.Header{
				ID: map[string]interface{}{
					"source":      fmt.Sprintf("%s", combo.source),
					"sensorLevel": "0",
					"element":     fmt.Sprintf("%s", combo.element),
				},
				Extra: map[string]interface{}{},
			},
			Observations: observations,
		})
	}

	dataset := dset.Dataset{
		TSeriesType: "frost0",
		TSeries:     tseries,
	}

	// serialize dataset
	dataset2, err := json.Marshal(dataset)
	if err != nil {
		return rrt.GetTestSequence{}, fmt.Errorf("failed to serialize dataset: %v", err)
	}

	// --- STEP 2: create the request/response pairs ------

	rrPairs := []rrt.ReqRespPair{}

	path := "/api/v1/obs/frost0/get"
	baseQuery := "?incobs=true&time=2020-01-01T00:00:00Z/2020-01-01T00:00:02Z"

	// define expected status codes (i.e. 200 Found or 404 Not Found) for different
	// elements/sources combinations
	for _, combo := range []struct {
		elements, sources string
		statusCode        int
	}{
		{"", "", 200},
		{"*", "*", 200},
		{"e0", "s0", 200},
		{"e*0", "s0", 200},
		{"e*0", "*", 200},
		{"e*0", "", 200},
		{"ex0,e*0", "s0", 200},
		{",,", "s0", 200},
		{",,", "*s*0*", 200},
		{"ex0", "s0", 404},  // no elements match
		{"e*0", "sx0", 404}, // no sources match
		{"", "*sx0*", 404},  // no sources match
		//add other interesting combos ... (if any?)
	} {
		esQuery := fmt.Sprintf("&elements=%s&sources=%s", combo.elements, combo.sources)
		rrPairs = append(rrPairs, rrt.ReqRespPair{
			Description: fmt.Sprintf("'%s&' -> status code %d", esQuery, combo.statusCode),
			Request: rrt.GetRequest{
				Path:  path,
				Query: baseQuery + esQuery,
			},
			ExpectedResponse: rrt.Response{
				StatusCode: combo.statusCode,
				Body:       `{}`,
			},
		})

	}

	// STEP 3: create GetTestSequence instance ------

	seq := rrt.GetTestSequence{
		Name: "query param wildcard and case sensitivity",
		Init: rrt.InitInfo{
			ClearPath:    "/api/v1/obs/clear",
			TsCreatePath: "/api/v1/obs/frost0/ts/create",
			PutPath:      "/api/v1/obs/frost0/put",
			Dataset:      string(dataset2),
		},
		ReqRespPairs: rrPairs,
	}

	return seq, nil
}

func createTestSequences() ([]rrt.GetTestSequence, error) {
	var seqs []rrt.GetTestSequence
	var seq rrt.GetTestSequence
	var err error

	seq, err = createTestQueryParamWildcardAndCaseSensitivity()
	if err != nil {
		return nil, fmt.Errorf("createTestQueryParamWildcardAndCaseSensitivity() failed: %v", err)
	}
	seqs = append(seqs, seq)

	// add more sequences ...

	return seqs, nil
}

// Test tests basic features for time series type 'frost0'.
func Test(t *testing.T, urlBase string, internalFrost bool) {
	seqs, err := createTestSequences()
	if err != nil {
		t.Errorf("createTestSequences() failed: %v", err)
		return
	}

	for _, seq := range seqs {
		t.Run(seq.Name, func(t *testing.T) { rrt.RunGetTestSequence(t, urlBase, seq) })
	}
}
