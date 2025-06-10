# features-go

[![Go Reference](https://pkg.go.dev/badge/github.com/altipla-consulting/features-go.svg)](https://pkg.go.dev/github.com/altipla-consulting/features-go)

Feature flags.


## Install

```shell
go get github.com/altipla-consulting/features-go
```


## Usage

### Configure features

To configure features, you need to run features configure, passing the server URL and the project corresponding to the flags.

```go
func main() {
  if err := features.Configure("https://youserver.com", "project"); err != nil {
    log.Fatal(err)
  }
}
```

### Verify feature flag

```go
if features.Flag(ctx, "feature") {
    fmt.Print("Feature flag is active.")
}
```

### Verify feature flag with tenant

```go
if features.Flag(ctx, "feature", features.WithTenant("tenant")) {
    fmt.Print("Feature flag is active with tenant.")
}
```


## Contributing

You can make pull requests or create issues in GitHub. Any code you send should be formatted using `make gofmt`.


## License

[MIT License](LICENSE)
