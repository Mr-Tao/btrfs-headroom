# Arch packaging

This directory is the source of truth for the two AUR package bases:

- `stable/` publishes `btrfs-headroom`.
- `git/` publishes `btrfs-headroom-git`.

Each directory contains the exact `PKGBUILD` and generated `.SRCINFO` copied
to its AUR Git repository. Regenerate `.SRCINFO` with
`makepkg --printsrcinfo` after every metadata change.

Publish the stable package only after the matching signed upstream tag exists,
so its source checksum can be verified rather than skipped.
