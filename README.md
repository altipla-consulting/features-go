# features-go

[![Go Reference](https://pkg.go.dev/badge/github.com/altipla-consulting/features-go.svg)](https://pkg.go.dev/github.com/altipla-consulting/features-go)

Feature Flags Go client.


## Install

```shell
go get github.com/altipla-consulting/features-go
```


## Usage

### Configure features (single project)

To configure features, run `Configure` with the server URL and the project corresponding to the flags.

```go
func main() {
  features.Configure("https://youserver.com", "project")
}
```

### Check feature flag is enabled

```go
if features.Flag("feature") {
    fmt.Print("Feature flag is enabled.")
}
```

### Check feature flag is enabled with tenant

```go
if features.Flag("feature", features.WithTenant("tenant")) {
    fmt.Print("Feature flag is enabled with tenant.")
}
```

### Multiple projects (connector / intermediary)

When the same process evaluates flags for several projects, use a `Pool`. It creates one client per project on first use and reuses it afterwards, so the in-memory cache and background sync are preserved across calls.

```go
pool := features.NewPool("https://youserver.com", features.WithLogger(log))

if pool.Flag(project, "feature", features.WithTenant(tenant)) {
    fmt.Print("Feature flag is enabled.")
}
```


## Contributing

You can make pull requests or create issues in GitHub. Any code you send should be formatted using `make gofmt`.


## License

[MIT License](LICENSE)
