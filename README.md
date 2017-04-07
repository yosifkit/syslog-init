# syslog-init

[![Build Status](https://travis-ci.org/yosifkit/syslog-init.svg?branch=master)](https://travis-ci.org/yosifkit/syslog-init)

## what is `syslog-init`

`syslog-init` is a simple `init` like [`tini`](https://github.com/krallin/tini) or [dumb-init](https://github.com/Yelp/dumb-init) to start a single child process, and wait for it to exit while reaping zombies and performing signal forwarding. As you might guess the main difference is the ability to request a syslog listener that will proxy the syslog messages to `stdout`.

## why do I need `syslog-init`?

There are certain applications, like HAProxy, that can only log to syslog. To simplify our container deployment, we don't want to also run a full syslog server (either in another container or complicate our HAProxy container with a full supervisor). The standard for container setups is to log to stdout so that they can be processed though something like `docker logs` or redirected to a log driver.

See https://github.com/docker-library/haproxy/pull/39 for why we don't just use busybox `syslogd` backgrounded from a shell script.  The short version is that the container won't exit with a `SIGTERM`, but will stop haproxy from listening.

If you are already running a syslog server, then you don't need `syslog-init` and should just point the haproxy container config to log there.

## using `syslog-init`

**TLDR:** get the `syslog-init` binary into your image and in the `PATH`. Set `SYSLOG_SOCKET` environment variable to `/dev/log`. Set `ENTRYPOINT` to `["syslog-init", "my-entrypoint"]` and ensure `CMD` is using the same json syntax.

### building `syslog-init`

Build the binary. Dockerfiles are provided for a stable and consistent build environment.  Either `Dockerfile.debian` or `Dockerfile.alpine` will work and create a static binary. _At some point, there will probably be signed releases._

```console
$ docker build -t yosifkit/syslog-init -f Dockerfile.debian .
```

### get `syslog-init` out of the image

Copy it to where it is needed. `docker cp` can be used once a container is running:

```console
# start a container
$ docker run -d --name sysl yosifkit/syslog-init sleep 1000

# cp the syslog-init binary to the current directory
$ docker cp sysl:/go/bin/syslog-init ./

# clean up the container
$ docker rm -f sysl
```

Alternatively we could just use tar to pipe the file to the host:

```console
$ docker run -i --rm syslog-init tar -c -C /go/bin/ syslog-init | tar -x -C ./
```

### add `syslog-init` to my project

Assuming that you current Dockerfile ends something like this:

```Dockerfile
# other build steps....

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["haproxy", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]

```

Just change/add the following and you'll have a syslog listener available at `/dev/log`.

```Dockerfile
ENV SYSLOG_SOCKET /dev/log
COPY syslog-init /usr/local/bin/
ENTRYPOINT ["syslog-init", "/docker-entrypoint.sh"]
CMD ["haproxy", "-f", "/usr/local/etc/haproxy/haproxy.cfg"]
```
