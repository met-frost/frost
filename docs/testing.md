# Testing

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
