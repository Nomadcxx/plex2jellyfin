# Plex2Jellyfin Authentication

## Configuration

Add to ~/.config/plex2jellyfin/config.toml:

password = "your-password"
secure_cookies = true

## Security Features

- Constant-time password comparison
- 32-byte random session tokens
- HttpOnly cookies
- 24-hour session expiration
- 1MB request body limit

## API

- GET /api/v1/auth/status
- POST /api/v1/auth/login
- POST /api/v1/auth/logout
