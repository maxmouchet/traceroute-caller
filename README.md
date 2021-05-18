# traceroute-caller
[![Version](https://img.shields.io/github/tag/m-lab/traceroute-caller.svg)](https://github.com/m-lab/traceroute-caller/releases) [![Build Status](https://travis-ci.com/m-lab/traceroute-caller.svg?branch=master)](https://travis-ci.com/m-lab/traceroute-caller) [![Coverage Status](https://coveralls.io/repos/m-lab/traceroute-caller/badge.svg?branch=master)](https://coveralls.io/github/m-lab/traceroute-caller?branch=master) [![GoDoc](https://godoc.org/github.com/m-lab/traceroute-caller?status.svg)](https://godoc.org/github.com/m-lab/traceroute-caller) [![Go Report Card](https://goreportcard.com/badge/github.com/m-lab/traceroute-caller)](https://goreportcard.com/report/github.com/m-lab/traceroute-caller)

## Local Development

You can clone the traceroute-caller repo on your development
workstation to edit, compile, and test it.  However, note that because
`paris-traceroute` is included as a Git submodule, you have to use the
`--recursive` flag when cloning the repo or run the following `git` command
inside the repo after it is cloned.

```sh
git submodule update --init --recursive
```

The safest way to build `traceroute-caller` is to use the repo's
`Dockerfile` to build a container image and then copy out the
`traceroute-caller` binary.

```sh
docker build --target build_caller --tag traceroute-caller:latest .
docker run -it --name trc:latest --entrypoint /bin/bash traceroute-caller
docker cp trc:/go/bin/traceroute-caller .
```

## Local Testing

Using `docker-compose` you can run a local instance of traceroute-caller that
operates in concert with events from `measurementlab/tcpinfo` and using
annotation from `measurement-lab/uuid-annotator`.

You must have a recent version of the Docker server configured and running in
your local environment. As well, your local environment must include a recent
version of `docker-compose`.

```sh
$ docker-compose version
docker-compose version 1.27.4, build 40524192
docker-py version: 4.3.1
CPython version: 3.7.7
OpenSSL version: OpenSSL 1.1.1g  21 Apr 2020
```

In the root directory of traceroute-caller, start a local build using
sample files in `./testdata`.

```sh
docker-compose up
```

This will create and run three containers.  Container names are prefixed by the
current working directory name (i.e., traceroute-caller).  After the containers
are running, trigger a network connection from within one of those containers.
For example:

```sh
docker exec -it traceroute-caller_traceroute-caller_1 apt-get update
```

The logs from traceroute-caller should indicate that files are being saved
under `./local/*`.

```sh
ls -lR ./local
```

Use `docker-compose down` to stop the containers and remove resources before
restarting your docker-compose environment.

```sh
docker-compose down
docker-compose up
```
