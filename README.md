[![DOI](https://zenodo.org/badge/1232731673.svg)](https://doi.org/10.5281/zenodo.22708527)

# Test the service

## System requirements

- [just](https://github.com/casey/just) 1.40.0 or later
- [docker](https://github.com/docker) 29.4.0 or later
- Python 3, available as `python`, with the `requests` package installed

## Test quickly via docker containers and Python scripts

Enter the following command from the root directory of the repository for a quick way to build,
run, and test the service:

```text
just test
```

This command builds and runs two docker containers: one for the Frost API and one for
the Postgres storage backend. It then runs all three validation cases in order:

1. The original CSV round-trip: ingest 2 series / 15 observations and verify the
	latest observation from each buoy in [output.csv](output.csv).
2. The fixed one-week Oslofjord radius query: ingest 3 series / 33 observations
	and validate 2 series / 16 observations, interval boundaries, and ordering.
3. Malformed ingestion: require HTTP 400 for an invalid JSON structure and verify
	that stored observations are unchanged.

Run an individual case with `just test-roundtrip`, `just test-week`, or
`just test-invalid-ingestion`.
The cases use separate source filters, so they can share the database and be
rerun without deleting their fixtures. Only use a test instance; fixture data
remains stored after testing. See [testing.md](docs/testing.md) for complete
requests and expected results.

Run just `just` (pun intended) to see other ways of running the command:

```text
just
```

## Other approaches to testing

See [testing.md](docs/testing.md) for other approaches to testing the code.
