package testabout

import (
	"testing"

	rrt "gitlab.met.no/frost/frost/test/integrationtests/reqresptest"
)

func createGetTests() []rrt.GetTest {
	var getTests []rrt.GetTest

	// add test
	getTests = append(getTests, rrt.GetTest{
		Name: "about",
		Request: rrt.GetRequest{
			Path: "/api/v1/about",
		},
		ExpectedResponse: rrt.Response{
			StatusCode: 200,
			Body:       "{}",
		},
	})

	// add more tests ...

	return getTests
}

// Test is an example of using the generic request/response testing framework in reqresptest.
func Test(t *testing.T, urlBase string, internalFrost bool) {

	getTests := createGetTests()

	for _, gtest := range getTests {
		t.Run(gtest.Name, func(t *testing.T) { rrt.RunGetTest(t, urlBase, gtest) })
	}
}
