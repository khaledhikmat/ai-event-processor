
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

**Please note** that the session defaults to `session123`. This session must be created!! Otherwise, you get an error message indicating that a session does not exist. Please run the `client.go` first to create the `session123` session.