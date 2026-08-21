# Third-Party Notices

Dark Forge builds upon the following open-source projects. Their licenses
apply to the respective portions of this distribution.

## Visual Studio Code — MIT
Copyright (c) 2015 - present, Microsoft Corporation.
The IDE shell is built from the VSCode source tree (pinned in `ide/upstream/`).
https://github.com/microsoft/vscode/blob/main/LICENSE.txt

## VSCodium — MIT
Copyright (c) 2021 - present, VSCodium.
Community build scripts and telemetry-free configuration for VSCode binaries
(`ide/` is derived from the VSCodium repository).
https://github.com/VSCodium/vscodium/blob/master/LICENSE

## Open VSX Registry
The default extension gallery is Open VSX (https://open-vsx.org), operated by
the Eclipse Foundation. Extension content served by Open VSX is subject to its
own license terms.

## Go dependencies (forge backend)
Each Go module in `forge/go.sum` carries its own license (MIT, Apache-2.0 or
BSD, see module repositories). Notable:
- golang.org/x/sys — BSD-3-Clause
- github.com/gorilla/websocket — BSD-2-Clause
- gopkg.in/yaml.v3 — Apache-2.0
- github.com/charmbracelet/* (bubbletea, bubbles, lipgloss) — MIT

## npm dependencies (extension)
Dev-only build tooling (@vscode/vsce, typescript); MIT licensed.
