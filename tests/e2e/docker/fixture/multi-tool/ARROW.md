# Multi Tool

Released on the stable channel as `stable-<version>` tags from a
container-local git host that is reachable only by cloning: it serves no
commit SHA, and the channel name `stable` is no git ref there. The steps
record the `${REF}` they ran for, which must be the release tag. An
identity with a marker under `/tmp/preinstalled/` was installed by something
other than Quiver, the way a client that installed itself is.

```arrow
schema: "arrow@v0"

metadata:
  name: e2e.multi-tool
  description: Stable-channel fixture whose steps record the release they ran for

targets:
  "linux/*":
    lifecycle:
      preinstalled:
        - type: run
          title: Looking for an install Quiver did not make
          command: test -f "/tmp/preinstalled/${ARROW_NAMESPACE}"
          timeout: 5s
          exit_on_failure: true

      install:
        - type: run
          title: Install
          command: echo "${REF}" > "${WORKDIR}/install-ref"
          timeout: 10s
          exit_on_failure: true

      update:
        - type: run
          title: Update
          command: echo "${REF}" >> "${WORKDIR}/update-refs"
          timeout: 10s
          exit_on_failure: true

      uninstall:
        - type: run
          title: Uninstall
          command: rm -f "${WORKDIR}/install-ref" "${WORKDIR}/update-refs"
          timeout: 10s
          exit_on_failure: false
```

Release history:
