# website-ssh

A terminal version of [miro.zip](https://miro.zip), served over SSH:

```sh
ssh miro.zip
```

Arrow keys or `hjkl` move around, `enter` or `space` opens a section or a
link, `q` quits.

## How it works

A small Go program built on Charm's [Wish](https://github.com/charmbracelet/wish)
(SSH server) and [Bubble Tea](https://github.com/charmbracelet/bubbletea)
(terminal UI). It is not OpenSSH and has no shell: every session goes
straight to the TUI, and the connection closes when the TUI exits.

Content is read from the website, not duplicated. On start and every 10
minutes the server fetches `index.html` and `projects.html` and parses them:

- Headings with an `id` (`#welcome`, `#skills`, `#projects`) become pages.
- Paragraphs, `h3` entries and project rows become text, and every `<a>`
  becomes a selectable link.
- The `.social-item` links become a "Links" page.

Website edits show up within 10 minutes without a redeploy. If the site is
unreachable, the copies in `snapshot/` (compiled in) are used. Refresh them
with `cp ../website/{index,projects}.html snapshot/`.

Text and link targets from the site are stripped of control characters, and
only `http(s)` and `mailto` links are kept, because everything is written
straight to visitors' terminals.

**Opening links:** a server can't open a browser on the visitor's machine, so
a link is copied to the clipboard (OSC 52, supported by most modern
terminals) and shown in the status line. Links are also OSC 8 hyperlinks, so
ctrl+click (cmd+click on macOS) works in most terminals.

## What visitors can't do

Anyone can connect without a password or key, like a public website, but:

- **Shell or commands:** none. `ssh miro.zip <command>` and sessions without
  a terminal (`ssh -T`) are refused.
- **Files:** no SFTP or SCP. The container runs as a non-root user on a
  read-only filesystem with no shell or tools (distroless).
- **Forwarding:** `-L`, `-R` and `-A` are refused.
- **Flooding:** connections are rate limited per IP (1 per second, burst of
  10), at most 50 sessions run at once, idle sessions close after 10 minutes
  and every session closes after an hour.

## Running locally

Needs podman (or Go 1.26). `make help` lists the shortcuts:

```sh
make run    # terminal 1: build and start the server on port 2222
make ssh    # terminal 2: connect to it
```

`make run-local` reads content from `../website/serve.sh` (port 8000) instead
of the live site. Change the port with `make run PORT=2300` and
`make ssh PORT=2300`.

Without a key file at `HOST_KEY_PATH` the server uses a temporary host key,
new on every start, so skip the known-hosts check while testing:
`ssh -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null localhost`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `LISTEN_ADDR` | `:2222` | Address to listen on |
| `HOST_KEY_PATH` | `.ssh/id_ed25519` | SSH host key, temporary one used if missing |
| `SITE_URL` | `https://miro.zip` | Website to read content from |
| `COUNTERS_URL` | (off) | Counter API, e.g. `https://miro.zip/api`. Each session counts as a visit |

## Deployment

Pushing to `main` builds a multi-arch image (`linux/arm64`, `linux/arm/v7`)
to `ghcr.io/miro-ali-akbar/website-ssh` via `.github/workflows/image.yml`. The
package is public, so the cluster pulls it without a secret.

It runs on `pi3-2` from the cluster repo (`apps/website-ssh/`). The pod
listens on port 2222 through a `hostPort`, and the router forwards external
TCP port 22 to `192.168.0.176:2222`. The nodes' own sshd stays on 22 and is
not exposed. The host key is a fixed SOPS-encrypted Secret, so returning
visitors don't get a "host key changed" warning after a redeploy. The
`deploy/` folder holds the same manifests without the encrypted key.

The pod requests 16 MiB of memory (limit 64 MiB). Internet scanners find
port 22 within minutes, so the logs fill with refused bot connections.
