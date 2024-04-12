# Use `simplestream-maintainer` to automate image server maintenance

`simplestream-maintainer` is capable of generating a simple streams product catalog and removing
expired or invalid product versions. However, it is a simple CLI tool that has to be invoked every
time an action needs to be done.

To automate the process of maintaining a simple streams image server, we recommend triggering the
build and prune commands periodically, either via cron jobs or systemd units.

On servers that host hundreds or more images, the build process can take quite a long time because
it has to calculate missing hashes and generate missing delta files. In such cases, we recommend
using systemd units to prevent triggering unnecessary builds if the previous build has not finished
yet.

## Example using systemd units

First create a systemd service file `/etc/systemd/system/simplestream-maintainer.service` which
runs `simplestream-maintainer` `build` and `prune` commands as in the provided example below.
Ensure `<simplestream_dir>` is replaced with an actual simple streams root directory and
`<simplestream_user>` with a user that has write permissions to that directory.

```sh
# /etc/systemd/system/simplestream-maintainer.service
[Unit]
Description=Simplestream maintainer
ConditionPathIsDirectory="<simplestream_dir>/images"

[Service]
Type=oneshot
User="<simplestream_user>"
Environment=TZ=UTC

# Commands are executed in the exact same order as specified.
ExecStart=simplestream-maintainer build "<simplestream_dir>" --logformat json --loglevel warn --workers 4
ExecStart=simplestream-maintainer prune "<simplestream_dir>" --logformat json --loglevel warn --retain-builds 3 --dangling

# Processes running at "idle" level get CPU time only when no one else needs it.
# This prevents simplestream-maintainer from consuming the computational resources when
# they are used to serve the images.
CPUSchedulingPolicy=idle

# Processes running at "idle" level get I/O time only when no one else needs the disk.
# This prevents simplestream-maintainer from consuming the disk I/O when it is used to
# serve the images.
IOSchedulingClass=idle
```

To start the systemd service periodically, create a new systemd timer file
`/etc/systemd/system/simplestream-maintainer.timer`. The following example triggers the previously
created systemd service each hour with a random 5 minute offset.

```sh
# /etc/systemd/system/simplestream-maintainer.timer
[Unit]
Description=Simplestream maintainer timer

[Timer]
OnCalendar=hourly
RandomizedDelaySec=5m
Persistent=true

[Install]
WantedBy=timers.target
```
