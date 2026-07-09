# Plex2Jellyfin Deployment Guide

## Quick Start

1. Build: make build
2. Configure: edit ~/.config/plex2jellyfin/config.toml
3. Run: ./plex2jellyfin serve --addr :8765

## HTTPS with Nginx

server {
    listen 443 ssl;
    proxy_pass http://localhost:8765;
}

Enable secure cookies in config.toml:
secure_cookies = true
