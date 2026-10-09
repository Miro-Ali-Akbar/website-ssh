# website-ssh

A terminal version of [miro.zip](https://miro.zip), served over SSH:

```sh
ssh miro.zip
```

Arrow keys or `hjkl` move around, `enter` or `space` opens a section or a
link, `q` quits.

## How it works

The server is a small Go program built on Charm's
[Wish](https://github.com/charmbracelet/wish) (SSH server) and
[Bubble Tea](https://github.com/charmbracelet/bubbletea) (terminal UI). It
is not OpenSSH, and there is no shell behind it: every SSH session is
handed straight to the TUI, and when the TUI exits the connection closes.

Content is not duplicated. On start, and every 10 minutes after, the
server fetches `index.html` and `projects.html` from the website and parses
them into sections:

- Headings with an `id` (`#welcome`, `#skills`, `#projects`) become pages.
- Paragraphs, `h3` entries and project rows become text, and every `<a>`
  becomes a selectable link.
- The `.social-item` links (GitHub, LinkedIn) become a "Links" page.

Editing the website updates the terminal version within 10 minutes, with
no redeploy. If the site can't be reached, the server falls back to the
copies in `snapshot/`, which are compiled into the binary. Refresh them now
and then with `cp ../website/{index,projects}.html snapshot/`.

### Opening links

A server can't open a browser on the visitor's computer. When a link is
opened, two things happen instead:

- The URL is copied to the visitor's clipboard with an OSC 52 escape code.
  This works in most modern terminals (kitty, WezTerm, iTerm2, Windows
  Terminal, foot, Alacritty, tmux with `set-clipboard on`).
- The URL is shown in the status line.

Every link is also drawn as an OSC 8 hyperlink, so in most terminals
ctrl+click (cmd+click on macOS) opens it directly.

## What visitors can't do

- **Shell or commands:** there is no shell. `ssh miro.zip <command>` is
  refused, and so is a session without a terminal (`ssh -T`).
- **Files:** there's no SFTP or SCP subsystem. In the container the
  program runs as a non-root user on a read-only filesystem with no shell
  or tools in the image (distroless).
- **Port or agent forwarding:** refused. The server registers no
  forwarding handlers, so `ssh -L`/`-R`/`-A` fail with "unsupported channel
  type".
- **Flooding:** connections are rate limited per IP (1 per second, burst of
  10). At most 50 sessions run at once, idle sessions close after 10
  minutes and every session closes after an hour.

Anyone can connect without a password or key, the same as a public
website.

## Running locally

Needs podman (or Go 1.26). `make help` lists the shortcuts:

```sh
make run    # terminal 1: build and start the server on port 2222
make ssh    # terminal 2: connect to it
```

`make run-local` reads content from `../website/serve.sh` (port 8000)
instead of the live site, to check website edits before pushing. Change the
port with `make run PORT=2300` and `make ssh PORT=2300`.

Then connect with `ssh -p 2222 localhost`. Without a key file at
`HOST_KEY_PATH` the server uses a temporary host key, new on every start,
so skip the known-hosts check while testing:
`ssh -p 2222 -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null localhost`.

Environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `LISTEN_ADDR` | `:2222` | Address to listen on |
| `HOST_KEY_PATH` | `.ssh/id_ed25519` | SSH host key, temporary one used if missing |
| `SITE_URL` | `https://miro.zip` | Website to read content from |
| `COUNTERS_URL` | (off) | Counter API, e.g. `https://miro.zip/api`. Each session counts as a visit |

## Deploying to the cluster

`miro.zip` already points at the home connection, so DNS needs no change.
SSH just needs a path from the internet to this server:

1. **Image:** pushing to `main` on GitHub builds a multi-arch image
   (`linux/arm64`, `linux/arm/v7`) to `ghcr.io/miro-ali-akbar/website-ssh`
   via `.github/workflows/image.yml`. Make the package public, or add an
   image pull secret to the cluster.
2. **Host key:** create one fixed key, so returning visitors don't get a
   "host key changed" warning after redeploys. Keep it out of git:

   ```sh
   ssh-keygen -t ed25519 -N "" -f id_ed25519
   kubectl create namespace website-ssh
   kubectl -n website-ssh create secret generic website-ssh-hostkey --from-file=id_ed25519
   ```
3. **Manifests:** copy `deploy/` to `rpi-cluster-gitops/apps/website-ssh/`
   in the cluster repo and add it to `apps/kustomization.yaml`. Flux picks
   it up from there.
4. **Router:** forward external TCP port 22 to `pi3-2` (192.168.0.176) port
   2222. The pod listens on 2222 through a `hostPort`, on the same node as
   `edge-proxy`, so all of miro.zip's ports go to one machine. The Pis' own
   sshd stays on port 22 and is not exposed to the internet.

The pod requests 16 MiB of memory (limit 64 MiB), which fits on `pi3-2`.

## Open questions

- Which architecture the Pi 3 nodes run (`uname -m` on `pi3-2`). The image
  covers both `arm64` and `armv7`, so this only matters if you want to
  build just one.
- Anyone scanning port 22 will now find this server, so the logs will
  fill with bot connection attempts. They fail harmlessly, but the logging
  middleware could be quieted if it gets noisy.
