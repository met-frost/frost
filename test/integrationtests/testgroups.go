// Package inttests ... TO BE DOCUMENTED.
package inttests

import (
	"testing"

	testabout "gitlab.met.no/frost/frost/test/integrationtests/routes/about"
	testobsbasicfrost0 "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/basic/frost0"
	testobstest2frost0 "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/test2/frost0"
	testobstest3frost0 "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/test3/frost0"
	testobstest4frost0 "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/test4/frost0"
	testobstsdeletefrost0 "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/tsdelete/frost0"
	testobsuldlfrost0 "gitlab.met.no/frost/frost/test/integrationtests/routes/obs/uldl/frost0"
)

type TestFunc func(*testing.T, string, bool)

type TestInfo struct {
	Name string `json:"name"`
	Func TestFunc `json:"function"`
}

type TestGroup struct {
	EnvVars   map[string]string `json:"environment"`
	TestInfos []TestInfo `json:"tests"`
}

// --- BEGIN definition of available test groups ----------------------------------------------
var testGroups = map[string]TestGroup{
	"local_all": {
		EnvVars: map[string]string{
			"OBSBACKEND":        "local",
			"FEATURES":          "frost0",
			"RESPONSEITEMLIMIT": "20",
		},
		TestInfos: []TestInfo{
			{"about", testabout.Test},
			{"obs/uldl/tstype=frost0", testobsuldlfrost0.Test},
			{"obs/test2/tstype=frost0", testobstest2frost0.Test},
			{"obs/test3/tstype=frost0", testobstest3frost0.Test},
			{"obs/test4/tstype=frost0", testobstest4frost0.Test},
			{"obs/basic/tstype=frost0", testobsbasicfrost0.Test},
			{"obs/tsdelete/tstype=frost0", testobstsdeletefrost0.Test},
		},
	},
	"postgres_all": {
		EnvVars: map[string]string{
			"OBSBACKEND":        "postgres",
			"FEATURES":          "frost0",
			"RESPONSEITEMLIMIT": "20",
			"PSBPASSWORD":       "mysecretpassword",
			"PSBALLOWCLEAR":     "true",
		},
		TestInfos: []TestInfo{
			{"about", testabout.Test},
			{"obs/uldl/tstype=frost0", testobsuldlfrost0.Test},
		},
	},
}
// --- END definition of available test groups ----------------------------------------------
