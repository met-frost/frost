package testobstest2frost0

import (
	"fmt"
	"os"
	"testing"

	rrt "gitlab.met.no/frost/frost/test/integrationtests/reqresptest"
)

func createGetTests() ([]rrt.GetTest, error) {
	var getTests []rrt.GetTest

	// Example 1: call clear function and upload a dataset
	getTests = append(getTests, rrt.GetTest{
		Name: "Example 2/1",
		Init: rrt.InitInfo{
			ClearPath:    "/api/v1/obs/clear",
			TsCreatePath: "/api/v1/obs/frost0/ts/create",
			PutPath:      "/api/v1/obs/frost0/put",
			Dataset: `
			{
				"tstype": "frost0",
				"tseries": [
					{
						"header": {
							"id": {
								"source": "mobile-source-1234",
								"sensorLevel": "0m",
								"element": "air_temperature"
							},
							"extra": {}
						},
						"observations": [
							{
								"time": "2020-06-16T06:00:00Z",
								"body": {
									"pos": {
										"lon": "-10.34",
										"lat": "60.12"
									},
									"value": "12.34"
								}
							},
							{
								"time": "2020-06-16T06:00:10Z",
								"body": {
								    "pos": {
									    "lon": "-10.382",
									    "lat": "60.19"
								    },
								    "value": "13.56"
							    }
							}
						]
					}
				]
			}
			`,
		},
		Request: rrt.GetRequest{
			Path: "/api/v1/obs/frost0/get",
			Query: "?incobs=true&time=2020-06-16T06:00:00Z/2020-06-16T06:00:20Z&" +
				"elements=air_temperature&sources=mobile-source-1234",
		},
		ExpectedResponse: rrt.Response{
			StatusCode: 200,
			Body: `
			{
				"data": {
				    "tstype": "frost0",
				    "tseries": [
					    {
					        "observations": [
						        {
							        "body": {
										"value": "13.56"
									}
					 	        }
					        ]
				        }
				    ]
				}
			}
			`,
		},
	})

	// Example 2: same as Example 1, but load the dataset from a file
	dataset, err := os.ReadFile("routes/obs/test2/frost0/dataset.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset from file: %v", err)
	}
	getTests = append(getTests, rrt.GetTest{
		Name: "Example 2/2",
		Init: rrt.InitInfo{
			ClearPath:    "/api/v1/obs/clear",
			TsCreatePath: "/api/v1/obs/frost0/ts/create", // <<<< apparently this fails to create
			// the time series, since the /put request (on the line below) fails with:
			// "...time series not found in internal registry: time series id not found:..."
			// -> find out why!
			PutPath: "/api/v1/obs/frost0/put",
			Dataset: string(dataset),
		},
		Request: rrt.GetRequest{
			Path: "/api/v1/obs/frost0/get",
			Query: "?incobs=true&time=2020-06-16T06:00:00Z/2020-06-16T06:00:20Z&" +
				"elements=air_temperature&sources=mobile-source-1234",
		},
		ExpectedResponse: rrt.Response{
			StatusCode: 200,
			Body: `
			{
				"data": {
				    "tstype": "frost0",
				    "tseries": [
					    {
					        "observations": [
						        {
							        "body": {
								        "value": "13.56"
							        }
					 	        }
					        ]
					    }
				    ]
				}
			}
			`,
		},
	})

	// Example 3: same as Example 2, but load the expected response body from a file too
	expBody, err := os.ReadFile("routes/obs/test2/frost0/expbody.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read expected response body from file: %v", err)
	}
	getTests = append(getTests, rrt.GetTest{
		Name: "Example 2/3",
		Init: rrt.InitInfo{
			ClearPath:    "/api/v1/obs/clear",
			TsCreatePath: "/api/v1/obs/frost0/ts/create",
			PutPath:      "/api/v1/obs/frost0/put",
			Dataset:      string(dataset),
		},
		Request: rrt.GetRequest{
			Path: "/api/v1/obs/frost0/get",
			Query: "?incobs=true&time=2020-06-16T06:00:00Z/2020-06-16T06:00:20Z&" +
				"elements=air_temperature&sources=mobile-source-1234",
		},
		ExpectedResponse: rrt.Response{
			StatusCode: 200,
			Body:       string(expBody),
		},
	})

	// Example 4: same as Example 3, but don't call clear function, create time series,
	// or upload a dataset (i.e. leave the Init field in rrt.GetTest to its default value with all
	// empty fields)
	getTests = append(getTests, rrt.GetTest{
		Name: "Example 2/4",
		Request: rrt.GetRequest{
			Path: "/api/v1/obs/frost0/get",
			Query: "?incobs=true&time=2020-06-16T06:00:00Z/2020-06-16T06:00:20Z&" +
				"elements=air_temperature&sources=mobile-source-1234",
		},
		ExpectedResponse: rrt.Response{
			StatusCode: 200,
			Body:       string(expBody),
		},
	})

	// add more tests ...

	return getTests, nil
}

// Test is an example of using the generic request/response testing framework in reqresptest.
func Test(t *testing.T, urlBase string, internalFrost bool) {

	getTests, err := createGetTests()
	if err != nil {
		t.Errorf("createGetTests() failed: %v", err)
		return
	}

	for _, gtest := range getTests {
		t.Run(gtest.Name, func(t *testing.T) { rrt.RunGetTest(t, urlBase, gtest) })
	}
}
