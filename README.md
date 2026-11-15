# hiwthi

**Hiwthi** ("Home Is Where The Heart Is", provisional name) sets up your home directory
from a description kept in a git repo of your own: shell setup, per-user tools, Python
versions, Flatpaks, GNOME settings and plain dotfiles. One description, every machine you
use. It complements [Waco](https://github.com/waco-org/waco), which sets up the system,
but doesn't need it.

*Work in progress: not usable yet.*

- Your description starts from [hiwthi-template](https://github.com/waco-org/hiwthi-template):
  `common.yml` for every home, `local.yml` for what a branch adds, `files/` for dotfiles.
- One executable, no root needed. It calls the tools that do the job (mise, uv, flatpak,
  gsettings) and downloads into your home what the system doesn't have.
- Commands (planned): `hiwthi init <repo>`, `apply`, `update`, `diff`, `check`,
  `capture`.

Design and decisions: [hiwthi-conversation](https://github.com/waco-org/hiwthi-conversation).

## Licence

[GPL-3.0](LICENSE), like Waco.
