
## Terminal 1 - Run the main server:

```bash
cd server
go run .
```

## Terminal 2 - Run the client:

```bash
cd client
go run main.go
```

## Browser:

```bash
http://localhost:8080
```

**Please note** that this defaults to `session123`. This session must be created!! Otherwise, the error message you get indicates that a session does noy exist. You can tun the `client.go` first to create the session.