# JellyWatch Deployment Guide

## Quick Start

1. Build: make build
2. Configure: edit ~/.config/jellywatch/config.toml
3. Run: ./jellywatch serve --addr :8765

## HTTPS with Nginx

server {
    listen 443 ssl;
    proxy_pass http://localhost:8765;
}

Enable secure cookies in config.toml:
secure_cookies = true
