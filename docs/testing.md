# Testing

## Docker and Python validation suite

From the repository root, with Docker, just, Python, and the existing `requests`
dependency available:

```sh
just test
```

`just test` builds and starts the Compose services once, then runs the original
CSV round-trip, weekly radius validation, and invalid-ingestion validation in
that order. Any failed request or assertion stops the recipe with a nonzero exit
status. The expected HTTP 400 in the negative case counts as a successful test.

| Recipe | Cases |
| --- | --- |
| `just test` | All three cases |
| `just test-roundtrip` | Original CSV / latest-observation round-trip |
| `just test-week` | Fixed weekly interval / radius query |
| `just test-invalid-ingestion` | Invalid JSON structure / unchanged data |

Every individual recipe also builds and starts the services. All cases target
`http://localhost:8080` by default. Use a test instance only: both positive cases
write observations and leave their series in storage. No test deletes data.
The original case queries only source `badevann.no`, buoys `004,011`, and
parameter `temperature`; the weekly case queries source `frost-e2e-week`.
These filters prevent the fixtures from interfering when the suite is rerun.
Standalone, unfiltered calls to [retrieve.py](../retrieve.py) still see both
fixtures.

### Case 1: CSV round-trip

`just test-roundtrip` runs [ingest.py](../ingest.py),
[retrieve.py](../retrieve.py), and [verify.py](../verify.py) scripts:

```sh
python ingest.py
python retrieve.py http://localhost:8080 output.csv sources=badevann.no buoyids=004,011 parameters=temperature
python verify.py
```

The fixed [input_example.csv](../input_example.csv) contains **2 series / 15
observations**. Ingestion creates the series via `/api/v1/obs/sst/ts/create` and
uploads their observations via `/api/v1/obs/sst/put`; both must return HTTP 200.
The complete retrieval request is:

```sh
curl --get 'http://localhost:8080/api/v1/obs/sst/get' \
  --data-urlencode 'incobs=true' \
  --data-urlencode 'time=latest' \
  --data-urlencode 'latestmaxage=PT1000000H' \
  --data-urlencode 'sources=badevann.no' \
  --data-urlencode 'buoyids=004,011' \
  --data-urlencode 'parameters=temperature'
```

Expected HTTP 200 and **2 series / 2 observations**, written to
[output.csv](../output.csv):

| Buoy ID | Latest timestamp | Value |
| --- | --- | --- |
| 004 | `2020-06-16T06:40:00Z` | `8.89` |
| 011 | `2020-06-16T07:30:00Z` | `9.10` |

Verification compares all CSV columns against the latest input row per buoy,
independent of output row order. It rejects missing, duplicate, unexpected, or
mismatched buoy rows. This original case uses the fixed local example write
token in [FrostImportUtils.py](../FrostImportUtils.py).

### Case 2: One week in an Oslofjord circle

Against an already running test instance, run `python test_ingestion.py week`.
For this case and Case 3, `--base-url http://localhost:8080` is optional, and
`FROST_WRITE_TOKEN` overrides the existing local example write token.

The fixed fixture is [input_week.csv](../test/examples/sst/input_week.csv), with
the same columns as [input_example.csv](../input_example.csv). It contains three
temperature series with 11 observations each. Rows are deliberately not in time
or buoy-ID order. The synthetic values are fixed, not live measurements.

The requested interval is exactly seven UTC days:
`2020-06-16T00:00:00Z/2020-06-23T00:00:00Z`. The geographic area is a circle in the
inner Oslofjord centered on longitude **10.73**, latitude **59.84**, with radius
**5 km**, applied to stationary positions in the series headers.

| Buoy ID | Longitude | Latitude | Weekly observations | In circle |
| --- | --- | --- | --- | --- |
| 001 | 10.73 | 59.84 | 8 | Yes (at center) |
| 002 | 10.75 | 59.85 | 8 | Yes (approximately 1.6 km away) |
| 003 | 10.00 | 59.00 | 8 | No (approximately 102 km away) |

The complete ingestion sequence is:

1. `POST http://localhost:8080/api/v1/obs/sst/ts/create`, with
   `Content-Type: application/json` and `X-Frost-Writetoken: <write token>`.
   Its body is `ingest.csv_to_dataset("test/examples/sst/input_week.csv")` with
   each series' `observations` replaced by `[]`.
2. `POST http://localhost:8080/api/v1/obs/sst/put`, with the same headers.
   Its body is the complete, unmodified result of that CSV conversion: root
   `{"tstype":"sst","tseries":[...]}`, all three headers, and all 33
   `{"time":"...","body":{"value":"..."}}` observations.

The test prints both complete JSON request bodies and the exact encoded request
URLs. It reports the content type but redacts the write token. Both POSTs must
return **HTTP 200**. The complete radius-filtered retrieval request is:

```sh
curl --get 'http://localhost:8080/api/v1/obs/sst/get' \
  --data-urlencode 'sources=frost-e2e-week' \
  --data-urlencode 'parameters=temperature' \
  --data-urlencode 'incobs=true' \
  --data-urlencode 'time=2020-06-16T00:00:00Z/2020-06-23T00:00:00Z' \
  --data-urlencode 'inside=[{"type":"circle","lon":10.73,"lat":59.84,"radius":5}]' \
  --data-urlencode 'geopostype=stationary' \
  --data-urlencode 'itemlimit=100'
```

Expected **HTTP 200**, `data.tstype == "sst"`, **2 time series** (001 and 002),
and **16 observations** (8 per series). Header IDs, names, positions, timestamps,
and values must match the fixture. The response fits one page: 2 headers plus
16 observations is 18 items, below `itemlimit=100`.

The test also makes two control GETs with the same source, parameter, `incobs`,
`geopostype`, and `itemlimit`, but without `inside`:

| Request time interval | Expected series | Expected observations |
| --- | --- | --- |
| `2020-06-15T00:00:00Z/2020-06-24T00:00:00Z` | 3 | 33 |
| `2020-06-16T00:00:00Z/2020-06-23T00:00:00Z` | 3 | 24 |

These controls prove that excluded observations and the outside buoy were
actually stored, rather than silently skipped during ingestion.

Temporal boundaries are **start-inclusive, end-exclusive**, `[start, end)`:

| Fixture timestamp (each buoy) | Included in weekly interval |
| --- | --- |
| `2020-06-15T23:59:59Z` (one second before start) | No |
| `2020-06-16T00:00:00Z` (exact start) | Yes |
| Midnight on June 17, 18, 19, 20, 21, and 22 | Yes (6 observations) |
| `2020-06-22T23:59:59Z` (one second before end) | Yes |
| `2020-06-23T00:00:00Z` (exact end) | No |
| `2020-06-23T00:00:01Z` (one second after end) | No |

**Ordering:** observations must be strictly ascending by time within each
series, regardless of CSV order. SST series order is **unspecified**, not sorted
by buoy ID or distance. The test matches series by ID instead of asserting array
order; it does not apply a client-side sort to actual observations.

### Case 3: Invalid ingestion JSON structure

Against an already running test instance, run `python test_ingestion.py invalid`.
`python test_ingestion.py all` runs Cases 2 and 3, not the original round-trip;
use `just test` for the full suite.

The failing request is syntactically valid JSON, but `tseries` is an object
instead of the required array:

```sh
curl --request POST 'http://localhost:8080/api/v1/obs/sst/put' \
  --header 'Content-Type: application/json' \
  --header "X-Frost-Writetoken: ${FROST_WRITE_TOKEN}" \
  --data '{"tstype":"sst","tseries":{"header":{},"observations":[]}}'
```

Set `FROST_WRITE_TOKEN` to the local example token from
[FrostImportUtils.py](../FrostImportUtils.py) or
your test instance's write token before running this manual request.

Expected **HTTP 400**, not a connection failure, authentication failure, or
HTTP 500. The JSON error contains `failed to validate request body` and
`tseries: Invalid type. Expected: array, given: object`. The negative test itself
passes only when this rejection occurs and the before/after weekly radius
retrievals have identical series, headers, and observations. It can also run
alone before the weekly fixture exists: the expected empty-source GET is then
HTTP 404 with `no matching time series found`.

## Direct, manual testing of local Frost instance

To quickly test specific aspects of a local Frost instance, it is often best to
inspect the behavior manually by either:

- opening and interacting with `http://localhost:8080` in a local web browser, or
- executing `curl "http://localhost:8080..." ...` with different arguments.

## Indirect testing with `go test`

For a more systematic and comprehensive testing, it is best to run pre-defined tests
by using the `go test` command. This is typically done automatically in the test stage of a
CI/CD pipeline, but it is also useful to run `go test` manually on the dev machine before pushing
a change (since then you would often save time by detecting many of the problems that would have
been detected by a CI/CD pipeline anyway).

### Basics

Below we list features of the `go test` command that are most relevant to Frost testing.
(But see also `go help test` and `https://golang.org/pkg/testing/` for more documentation about
Go testing.)

#### Specifying which tests to run

**Option 1:**

Run the following command from *anywhere in the repo directory tree* to only run tests directly
under the `test/x/y` directory:

```text
go test gitlab.met.no/frost/frost/test/x/y
```

**Option 2:**

Run the following command from a *specific directory* to run all tests in that directory tree:

```text
go test ./...
```

#### Invalidate cached test results

The following command invalidates cached test results:

```text
go clean -testcache
```

Example:

```text
go clean -testcache && (cd test/acceptancetests; go test ./...)
```

#### Color test output

Two options for coloring test output:

**Option 1** - using sed

Pipe `go test` through `sed`:

```text
... go test ... | sed ''/PASS/s//$(printf "\033[32mPASS\033[0m")/'' | sed ''/FAIL/s//$(printf "\033[31mFAIL\033[0m")/''
```

**Option 2** - using grc

Run `go test` through `grc`:

```text
... grc go test ...
```

Where `grc` is installed as follows:

```text
sudo apt install grc
```

Create a `~/.grc/grc.conf` with the following contents:

```text
# Go
\bgo.* test\b
conf.gotest
```

Create a `~/.grc/conf.gotest` with the following contents:

```text
regexp==== RUN .*
colour=blue
-
regexp=--- PASS: .*
colour=green
-
regexp=^PASS$
colour=green
-
regexp=.*passed.*
colour=green
-
regexp=^(ok|\?) .*
colour=magenta
-
regexp=--- FAIL: .*
colour=red
-
regexp=.*failed.*
colour=red
-
regexp=[^\s]+\.go(:\d+)?
colour=cyan
```

#### Pass environment variables

```text
... KEY1=val1 KEY2=val2 go test ...
```

### Integration testing of HTTP requests against a Frost instance

The special file `inttests_test.go` is a program designed to test Frost by running "external" HTTP
requests against a running Frost instance (*note:* for truly external testing, the test client
would have to be run outside the firewall etc.).

**NOTE:** In this case the `./...` option to `go test` is not necessary since all tests are
run from the single test function defined in `inttests_test.go`.

### Test groups

Tests are defined in the `testGroups` structure in `testgroups.go` which has the following
structure for each *test group*:

```text
    "environment": {
        <key 1>: <val 1>,
        <key 2>: <val 2>,
        ...
    },
    "tests": [
        {
            "name": <name 1>,
            "function": <function 1>
        },
        {
            "name": <name 2>,
            "function": <function 2>
        },
        ...
    ]
```

Only tests for a single test group may be run by a single `go test` execution. The test group must
be specified with the `TESTGROUP` environment variable, for example `TESTGROUP=postgres_all`.

**NOTE:** The pre-defined values for the test group environment variables may be overridden on the
command-line. So `... <key 1>=<overridden val 1> ...` will override any `<key 1>` environment
variable for the selected test group.

**NOTE:** The test group environment variables are only applicable when tests are run against an
internal Frost instance (since that's the only case where the test runner program is actually
starting the Frost instance itself).

#### List available tests

Set the `LISTTESTS` environment variable to 'true' to just list available tests for the selected
test group without executing them.

#### Run only certain tests

By default, all tests in the selected test group are run.

Set the `TESTS` environment variable to run only certain tests available for the test group.
The value is a comma-separated list of test names from the output of `LISTTESTS=true` (see above),
for example `TESTS=about,obs/basic/tstype=ranked`.

#### Run tests against an external Frost instance

In this mode, the Frost instance to test against is assumed to be already running. It can run on
any accessible host (including the same, local machine on which the `go test` command is run, but
that is likely to be less interesting).

- Pros:
  - You can test an existing Frost instance that can run anywhere.
  - You save time by not having to build and deploy the Frost instance.
- Cons:
  - You are stuck with whatever environment the Frost instance was started with, such as
    storage backend and features (see above section about test group environment variables).
  - You would potentially have to compete with other clients for the capacity of the Frost instance.
  - You don't get access to the stdout and stderr of the Frost instance.

The mode is specified by providing the environment variable `URLBASE`, for example like this:

```text
... URLBASE=https://frost-test-instance.met.no go test ...

```

or like this to test against a running Frost instance on the local machine (note the typical
use of `http` rather than `https` for the scheme/protocol in this case):

```text
... URLBASE=http://localhost:8080 go test ...
```

#### Run tests against an internal Frost instance

In this mode, the Frost instance to test against is built from the same repo as the test code
and also run on the same, local machine on which the `go test` command is run.

- Pros:
  - Compilation errors will be caught.
  - You can test different storage backends and initialize these specifically for this test run
    (see above section about test group environment variables).
  - You get access to the stdout and stderr of the Frost instance.
- Cons:
  - You cannot test an existing Frost instance.

Set the environment variable `VERBOSE=true` to dump the stdout and stderr from the Frost
instance. This can be useful for debugging.

The mode is specified by **not** providing the environment variable `URLBASE` to the `go test`
command. On the other hand, all environment variables are now automatically passed on to the
Frost instance when this is launched (see above section about test group environment variables).

#### Complete examples

##### Example 1: Run general tests against internal Frost w/ local (in-memory) storage backend

```text
go clean -testcache && (cd test/integrationtests; TESTGROUP=local_all grc go test -v)
```

**TIP:** Do this to just list relevant Frost environment and available tests:

```text
go clean -testcache && (cd test/integrationtests; TESTGROUP=local_all LISTTESTS=true grc go test)
```

##### Example 2: Run general tests against internal Frost w/ Postgres storage backend

```text
go clean -testcache && (cd test/integrationtests; TESTGROUP=postgres_all grc go test -v)
```

**TIP:** Do this to just list relevant Frost environment and available tests:

```text
go clean -testcache && (cd test/integrationtests; TESTGROUP=postgres_all LISTTESTS=true grc go test)
```

**NOTE:** This example requires a Postgres storage backend to be already running somewhere.
See this [README](../psb/README.md) for how to build and run one in a docker container.

#### Basic testing framework

For integration testing, a basic [testing framework](TestingFramework.md) is available.
