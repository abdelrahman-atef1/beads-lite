# beads-lite setup notes (sre, 2026-10-25)
- H:/Projects does not exist; projects live in E:/Projects. Clones: E:/Projects/beads-lite, E:/Projects/beads-lite-ui.
- Upstream bd: https://github.com/gastownhall/beads (installed @beads/bd 1.3.1 = tag v1.3.1). Remote `upstream`; branch `main` = upstream main as cloned (HEAD e866614a6); working branch `lite`.
- Upstream UI: https://github.com/mantoni/beads-ui (installed 0.12.6 = tag v0.12.6; cloned main is newer, b1d519a). Remote `upstream`.
- Private origins: github.com/abdelrahman-atef1/beads-lite and beads-lite-ui (verified private). Full history, all branches and tags pushed.
- Push uses SSH: gh's HTTPS token lacks the `workflow` scope (upstream has .github/workflows). Host keys verified against `gh api meta`:
  GIT_SSH_COMMAND='ssh -o UserKnownHostsFile=<file of gh api meta ssh_keys> -o StrictHostKeyChecking=yes'. Not changed: token scope, ~/.ssh/known_hosts.
- Both listed in ~/company/scripts/backup-projects.list.
- Spec Kit: specify-cli v1.2.3 (github/spec-kit tag, pip into venv C:/Users/abduu/tools/specify-venv; run Scripts/specify.exe). No account/key. Init: `specify init --here --force --non-interactive --integration claude --ignore-agent-tools` (claude not on PATH for the check) -> .specify/ and .claude/skills/speckit-*. No bd/beads tracking for this project (owner).
