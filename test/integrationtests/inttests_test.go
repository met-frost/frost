package inttests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.met.no/frost/frost/internal/common"
)

var frostCmd *exec.Cmd
var frostOut bytes.Buffer
var frostErr bytes.Buffer

func getRepoRootDir() (string, error) {
	_, currFileName, _, ok := runtime.Caller(1) // get absolute/full name of current file
	if !ok {
		return "", fmt.Errorf("runtime.Caller(1) failed")
	}
	return path.Join(path.Dir(currFileName), "../.."), nil // WARNING: update if file is moved!
}

// Builds and starts an instance of Frost on the local host.
//
// Returns nil upon success, otherwise error.
func runFrost() error {

	var err error

	repoRootDir, err := getRepoRootDir()
	if err != nil {
		return fmt.Errorf("getRepoRootDir() failed: %v", err)
	}

	frostCmd, err = common.BuildAndRunGoProgram(
		"main/main.go", repoRootDir, os.Environ(), &frostOut, &frostErr)
	if err != nil {
		return fmt.Errorf(
			"common.BuildAndRunGoProgram(Frost) failed: %v\n\tstdout: %v\n\tstderr: %v",
			err, frostOut.String(), frostErr.String())
	}

	return nil
}

// Stops the local Frost instance.
func stopFrost(t *testing.T) {

	_ = t // unused
	common.StopProgram(frostCmd)
}

func localTCPPortIsOpen(port int) bool {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err == nil {
		l.Close()
	}
	return err != nil
}

// Initializes acceptance testing by either 1) building, running and using a new Frost
// instance on the local host, or 2) using a Frost instance that is already running
// (typically on another host).
// The function returns four values:
//
//	1: whether the operation was successful
//	2: the URL base to use for sending requests to Frost
//	3: whether the Frost instance is an internal one that is built and run for this purpose
//	4: the function to call for disconnecting from Frost after testing is done
func connectFrost(t *testing.T) (bool, string, bool, func(t *testing.T)) {

	_ = t // unused

	if urlBase, ok := os.LookupEnv("URLBASE"); ok {
		if strings.HasSuffix(urlBase, "/") {
			log.Printf("'/' suffix not allowed in URLBASE: %s", urlBase)
			return false, "", false, func(t *testing.T) {}
		}
		log.Printf("using existing Frost instance (URL base: %s)", urlBase)
		return true, urlBase, false, func(t *testing.T) {}
	}

	log.Printf("building, running and testing against a new Frost instance on the local machine")

	if localTCPPortIsOpen(8080) {
		log.Printf("TCP port 8080 already in use")
		return false, "", true, func(t *testing.T) {}
	}

	err := runFrost()
	if err != nil {
		log.Printf("runFrost() failed: %v", err)
		return false, "", true, func(t *testing.T) {}
	}

	return true, "http://localhost:8080", true, func(t *testing.T) { stopFrost(t) }
}

func awaitFrostReady(urlBase string) error {
	maxAttempts := 10
	log.Printf("await Frost ready (within %d secs) ...", maxAttempts)

	started := false
	for i := 0; (!started) && (i < maxAttempts); i++ {

		// send GET request to 'api/v1/about'
		url := fmt.Sprintf("%s/api/v1/about", urlBase)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf(
				"request to %s failed (attempt %d of %d): %v; trying again in one second\n",
				url, i+1, maxAttempts, err)
			time.Sleep(1 * time.Second) // try again in one second
		} else {
			if resp.StatusCode == 200 {
				started = true
			}
		}
	}

	if !started {
		msg := fmt.Sprintf("failed to await Frost ready within %d secs", maxAttempts)
		if frostCmd != nil { // provide additional output from local instance
			msg += fmt.Sprintf("\n\tstdout: %v\n\tstderr: %v", frostOut.String(), frostErr.String())
		}
		return errors.New(msg)
	}

	log.Printf("Frost ready!")

	return nil
}

func (t TestFunc) MarshalJSON() ([]byte, error) {
	funcName := runtime.FuncForPC(reflect.ValueOf(t).Pointer()).Name()
	return json.Marshal(funcName)
}

// getTestGroup() gets the test group to be used for this test run.
//
// Returns (test group name, test group, nil) upon success, otherwise (..., ..., error).
func getTestGroup() (string, TestGroup, error) {

	testGroup := strings.TrimSpace(common.Getenv("TESTGROUP", ""))
	tgKeys := []string{}
	for tgKey := range testGroups {
		tgKeys = append(tgKeys, tgKey)
		if tgKey == testGroup {
			return tgKey, testGroups[tgKey], nil
		}
	}

	return "", TestGroup{}, fmt.Errorf(
		"invalid TESTGROUP: >%s< (expected one of these: %s)",
		testGroup, strings.Join(tgKeys, ", "))
}

func Test(t *testing.T) {

	// --- PHASE 1: define tests --------------------------------------

	t.Logf("\n*** defining tests ***\n")

	tgName, testGroup, err := getTestGroup()
	if err != nil {
		t.Errorf("getTestGroup() failed: %v", err)
		return
	}

	// set environment variables defined by test group unless already defined (thus allowing
	// variables to be overridden from the command-line)
	for key, val := range testGroup.EnvVars {
		finalVal := val
		orVal := os.Getenv(key)
		if (orVal != "") && (orVal != val) {
			finalVal = orVal
			testGroup.EnvVars[key] = finalVal
		}
		if err := os.Setenv(key, finalVal); err != nil {
			t.Errorf("os.Setenv(%s, %s) failed: %e", key, finalVal, err)
			return
		}
	}

	if common.Getenv("LISTTESTS", "false") == "true" {

		// TODO: only list tests in the current test group

		s, err := json.Marshal(testGroup)
		if err != nil {
			t.Errorf("json.Marshal() failed: %v", err)
			return
		}

		fmt.Printf("relevant Frost exec environment and available tests "+
			"to run against Frost for test group %s:\n%s\n", tgName, s)
		return
	}

	// --- PHASE 2: initialize Frost service --------------------------------------

	t.Logf("\n*** initializing Frost service with final relevant environment: %v ***\n",
		testGroup.EnvVars)

	ok, urlBase, internalFrost, disconnectFrost := connectFrost(t)
	defer disconnectFrost(t)
	if !ok {
		t.Errorf("failed to initialize test case")
		return
	}

	if err := awaitFrostReady(urlBase); err != nil {
		t.Errorf("awaitFrostReady() failed: %v", err)
		return
	}

	// --- PHASE 3: run tests --------------------------------------
	testNames := common.ExtractCSVVals(common.Getenv("TESTS", ""))

	t.Logf("\n*** running tests in test group %s ***\n", tgName)

	for _, testInfo := range testGroup.TestInfos {
		if (len(testNames) == 0) || common.StringInStrings(testInfo.Name, testNames) {
			t.Run(testInfo.Name, func(t *testing.T) { testInfo.Func(t, urlBase, internalFrost) })
		}
	}

	// Useful for debugging in case we 1) use a local Frost instance that we build and run
	// ourselves, and 2) we suspect that the instance terminates prematurely for some reason:
	if common.Getenv("VERBOSE", "false") == "true" {
		if !internalFrost {
			t.Logf("WARNING: VERBOSE n/a when testing against an external Frost instance")
		} else {
			t.Logf("frostOut:\n%s\n", frostOut.String())
			t.Logf("frostErr:\n%s\n", frostErr.String())
		}
	}
}
