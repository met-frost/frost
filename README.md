# Test the service

## System requirements

- [just](https://github.com/casey/just) 1.40.0 or later
- [docker](https://github.com/docker) 29.4.0 or later

## Test quickly via docker containers and Python scripts

Enter the following command from the root directory of the repository for a quick way to build,
run, and test the service:

```text
just test
```

This command builds and runs two docker containers: one for the Frost API and one for
the Postgres storage backend. It then runs Python scripts to demonstrate client requests for
writing and reading a dataset.

Run just `just` (pun intended) to see other ways of running the command:

```text
just
```

## Other approaches to testing

See [testing.md](docs/testing.md) for other approaches to testing the code.
