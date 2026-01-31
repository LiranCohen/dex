# Poindexter Magic Deploy

One command. Two QR codes. Zero friction.

## The Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  $ curl -fsSL https://dex.sh | bash                            │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │                                                           │ │
│  │    █▀▀▀▀▀█ ▄▄▄▄▄ █▀▀▀▀▀█                                 │ │
│  │    █ ███ █ █▄▄▄█ █ ███ █    📱 SCAN TO JOIN TAILSCALE   │ │
│  │    █ ▀▀▀ █ ▀▀▀▀▀ █ ▀▀▀ █                                 │ │
│  │    ▀▀▀▀▀▀▀ ▀ ▀ ▀ ▀▀▀▀▀▀▀    Sign up or log in           │ │
│  │                                                           │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ✓ Connected as dex.yak-bebop.ts.net                           │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │                                                           │ │
│  │    █▀▀▀▀▀█ ▄▄▄▄▄ █▀▀▀▀▀█                                 │ │
│  │    █ ███ █ █▄▄▄█ █ ███ █    📱 SCAN TO COMPLETE SETUP   │ │
│  │    █ ▀▀▀ █ ▀▀▀▀▀ █ ▀▀▀ █                                 │ │
│  │    ▀▀▀▀▀▀▀ ▀ ▀ ▀ ▀▀▀▀▀▀▀    Enter API keys              │ │
│  │                                                           │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ✓ POINDEXTER IS READY                                         │
│                                                                 │
│    https://dex.yak-bebop.ts.net                                │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## What You Get

| Feature | Description |
|---------|-------------|
| **Auto HTTPS** | Tailscale provisions Let's Encrypt certs automatically |
| **Zero ports exposed** | Nothing on public internet, only your Tailscale network |
| **Passwordless SSH** | `tailscale ssh dex` - no keys to manage |
| **Mobile access** | Works from any device with Tailscale app |
| **Identity headers** | Tailscale tells dex who you are |

## Install

SSH into your target server and run:

```bash
curl -fsSL https://raw.githubusercontent.com/lirancohen/dex/master/scripts/install.sh | bash
```

That's it. Follow the QR codes.

## What Happens

### QR Code 1: Join Tailscale

The first QR code links to Tailscale's auth page. When you scan:

1. **New to Tailscale?** → Sign up (free, takes 30 seconds)
2. **Already have Tailscale?** → Log in with your account

Your phone joins the same network as the server. The server appears as `dex.your-tailnet.ts.net`.

### QR Code 2: Complete Setup

The second QR code opens a setup wizard running on the server. Because your phone is now on the same Tailscale network, it can access it securely.

The wizard:
1. Collects your **Anthropic API key** (for AI sessions)
2. Collects your **GitHub token** (for repo/PR management)
3. Redirects you to register a **passkey** (Face ID, Touch ID, or security key)

### After Setup

- Server runs dex as a systemd service
- Tailscale Serve proxies HTTPS to localhost
- Access from any device: `https://dex.your-tailnet.ts.net`

## Prerequisites

### On the Server
- Linux (Ubuntu, Debian, RHEL, Arch, etc.)
- Root/sudo access
- Internet connection

### On Your Phone
- Camera (for QR codes)
- Tailscale app (will be prompted to install if needed)

### API Keys You'll Need
- **Anthropic**: https://console.anthropic.com/settings/keys
- **GitHub**: https://github.com/settings/tokens (needs `repo` scope)

## How It Works (Technical Details)

### Step 1: Tailscale Authentication

```bash
tailscale up --hostname=dex --ssh
```

This outputs an auth URL like `https://login.tailscale.com/a/abc123`. The installer converts it to a QR code using `qrencode -t ANSI256`.

When you scan and authenticate:
- The server joins your tailnet as `dex`
- [Tailscale SSH](https://tailscale.com/kb/1193/tailscale-ssh) is enabled
- You get a MagicDNS name like `dex.yak-bebop.ts.net`

### Step 2: Setup Wizard

The installer runs `dex-setup`, a Go binary that:
- Serves a mobile-friendly web UI on localhost:9999
- Uses [Tailscale Serve](https://tailscale.com/kb/1312/serve) to expose it with HTTPS
- Collects API keys via the web form
- Writes secrets to disk and signals completion
- Redirects to the main app for passkey registration

Because you authenticated in Step 1, your phone is on the tailnet and can reach `https://dex.your-tailnet.ts.net`.

### Step 3: Service Configuration

```bash
# Install binary and frontend
/opt/dex/dex
/opt/dex/frontend/

# Create systemd service
/etc/systemd/system/dex.service

# Configure permanent HTTPS via Tailscale
tailscale serve --bg --https=443 http://127.0.0.1:8080
```

## After Installation

### Access Dex

From any device on your Tailscale network:
```
https://dex.your-tailnet.ts.net
```

### SSH Access

No keys needed - Tailscale handles authentication:
```bash
tailscale ssh dex
```

Or use the [web-based SSH console](https://tailscale.com/kb/1216/tailscale-ssh-console) in the Tailscale admin panel.

### Service Management

```bash
# Check status
sudo systemctl status dex

# View logs
sudo journalctl -u dex -f

# Restart
sudo systemctl restart dex
```

### Update Dex

```bash
tailscale ssh dex
curl -fsSL https://github.com/lirancohen/dex/releases/latest/download/dex-linux-amd64 -o /tmp/dex
sudo mv /tmp/dex /opt/dex/dex && sudo chmod +x /opt/dex/dex
sudo systemctl restart dex
```

## Troubleshooting

### "QR code not scanning"

- Make sure terminal window is large enough
- Try the URL shown below the QR code
- On some terminals, try: `TERM=xterm-256color` before running

### "Tailscale auth URL not appearing"

The server might already be connected. Check:
```bash
tailscale status
```

### "Can't reach setup wizard"

1. Is your phone on Tailscale? Check the Tailscale app.
2. Is the server connected? `tailscale status`
3. Try the IP directly: `https://100.x.y.z`

### "Passkey not working"

- Make sure you're using the same device/browser you registered with
- Try removing and re-adding the passkey in your device's settings
- If using iCloud Keychain, ensure it's synced
- Check that the domain in the URL matches exactly

## Security Model

- **Network isolation**: Dex is only accessible via Tailscale, not the public internet
- **Encrypted transport**: All traffic is WireGuard-encrypted
- **Identity verification**: [Tailscale identity headers](https://tailscale.com/kb/1312/serve#identity-headers) pass through user info
- **Passkey auth**: WebAuthn/FIDO2 for phishing-resistant authentication
- **No passwords**: Credentials bound to device biometrics, nothing to remember or steal

## Uninstall

```bash
sudo systemctl stop dex
sudo systemctl disable dex
sudo rm /etc/systemd/system/dex.service
tailscale serve reset
sudo rm -rf /opt/dex
# Optionally: sudo tailscale logout
```

## Sources

- [Tailscale Install](https://tailscale.com/kb/1031/install-linux)
- [Tailscale Serve](https://tailscale.com/kb/1312/serve)
- [Tailscale SSH](https://tailscale.com/kb/1193/tailscale-ssh)
- [Tailscale HTTPS Certificates](https://tailscale.com/kb/1153/enabling-https)
- [Identity Headers](https://tailscale.com/kb/1312/serve#identity-headers)
