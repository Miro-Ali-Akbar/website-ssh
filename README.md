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
