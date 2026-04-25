# Tool A (Markdown)

A simple tool fixture for testing ARROW.md format.

```arrow
schema: "arrow@v0"

metadata:
  name: quiver-test.tool-a-markdown
  version: 1.0.0
  description: Simple tool with no dependencies (ARROW.md format)

targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo installed-tool-a-markdown
          title: Install
          timeout: 10s
          exit_on_failure: true
      execute:
        - type: run
          command: echo executed-tool-a-markdown
          title: Execute
          timeout: 10s
          exit_on_failure: true
      uninstall:
        - type: run
          command: echo uninstalled-tool-a-markdown
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
```
