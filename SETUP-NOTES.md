# beads-lite setup notes (sre, 2026-10-25)
- Clones live side by side: beads-lite and beads-lite-ui.
- Upstream bd: https://github.com/gastownhall/beads (installed @beads/bd 1.3.1 = tag v1.3.1). Remote `upstream`; branch `main` = upstream main as cloned (HEAD e866614a6); working branch `lite`.
- Upstream UI: https://github.com/mantoni/beads-ui (installed 0.12.6 = tag v0.12.6; cloned main is newer, b1d519a). Remote `upstream`.
- Origins (PUBLIC since 2026-10-25, owner request): github.com/abdelrahman-atef1/beads-lite and beads-lite-ui. Full history, all branches and tags pushed. Upstream licences: beads MIT (Beads Contributors), beads-ui MIT (Maximilian Antoni); both LICENSE files kept unchanged. Scanned our added files (lite branch diff vs upstream main) for secrets/paths: none; machine paths scrubbed from these notes. Commits carry the owner git identity (name + email), public.
- Push uses SSH: gh's HTTPS token lacks the `workflow` scope (upstream has .github/workflows). Host keys verified against `gh api meta`:
  GIT_SSH_COMMAND='ssh -o UserKnownHostsFile=<file of gh api meta ssh_keys> -o StrictHostKeyChecking=yes'. Not changed: token scope, ~/.ssh/known_hosts.
- Backup: both are listed in the company backup list; because origin is public, each has a private `backup` remote (abdelrahman-atef1/backup-beads-lite, backup-beads-lite-ui) and the backup script pushes only there.
- Spec Kit: specify-cli v1.2.3 (github/spec-kit tag, pip into a local venv; run its Scripts/specify.exe). No account/key. Init: `specify init --here --force --non-interactive --integration claude --ignore-agent-tools` (claude not on PATH for the check) -> .specify/ and .claude/skills/speckit-*. No bd/beads tracking for this project (owner).
