# Lavender Messenger - macOS Client

Native macOS client for [Lavender Messenger](https://github.com/ferzferz11-sudo/msg) — a secure messaging application.

## Requirements

- Go 1.21+
- macOS 14+

## Building

```bash
go build -o lavender-macos .
```

## Running

```bash
go run .
```

## Configuration

Edit `config.yaml` to set server address and theme preferences:

```yaml
server_address: 13.140.25.249:50051
current_theme: dark
```

## Features

- Login with username/password
- User registration (new account creation)
- Server selector (new: `13.140.25.249:50051`, old: `159.195.38.145:50051`)
- Chat list with unread count badges
- Direct chat creation (click on user)
- Online status indicators
- Message history
- Reply quotes and reactions
- Emoji picker
- Light/dark theme toggle
- Custom avatar colors per user

## Version

Current: 1.0.1.28
