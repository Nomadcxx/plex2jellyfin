<div align="center">
  <img src="assets/plex2jellyfin-header.png" alt="plex2jellyfin" width="720" />
</div>

Your Plex library, renamed the way Jellyfin wants it. Migrate once, then a daemon keeps every new download clean.

[Documentation](https://nomadcxx.github.io/plex2jellyfin/docs/) · [GitHub](https://github.com/Nomadcxx/plex2jellyfin)

## Installation

### Option A — TUI installer

```bash
curl -sSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/install.sh | sudo bash
```

<details>
<summary><b>Option B — Build from source + CLI setup</b></summary>

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/scripts/fresh-build-install.sh)
plex2jellyfin setup
```

</details>

<details>
<summary><b>Option C — Build from source + web setup</b></summary>

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Nomadcxx/plex2jellyfin/main/scripts/fresh-build-install-web.sh)
```

Then open the URL the script prints (usually `http://127.0.0.1:5522/`).

</details>

<details>
<summary><b>Option D — Docker</b></summary>

```bash
docker compose -f docker-compose.example.yml up -d
```

See the [Docker guide](https://nomadcxx.github.io/plex2jellyfin/docs/getting-started/docker/).

</details>

<details>
<summary><b>Option E — Development</b></summary>

```bash
git clone https://github.com/Nomadcxx/plex2jellyfin.git
cd plex2jellyfin
go build -o installer ./cmd/installer && sudo ./installer
```

</details>

<details>
<summary><b>Option F — AUR (Arch Linux)</b></summary>

Coming soon.

</details>

<details>
<summary><b>Option G — Deb / RPM</b></summary>

Packages on [GitHub Releases](https://github.com/Nomadcxx/plex2jellyfin/releases/latest). Setup steps: [packages](https://nomadcxx.github.io/plex2jellyfin/docs/getting-started/packages/).

</details>

The companion [Jellyfin plugin](https://github.com/Nomadcxx/plex2jellyfin-plugin) is required for the feedback loop; setup wizards can install it. Details: [plugin docs](https://nomadcxx.github.io/plex2jellyfin/docs/getting-started/jellyfin-plugin/).

## License

GPL-3.0-or-later
