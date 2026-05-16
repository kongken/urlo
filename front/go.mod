// This frontend package is intentionally a separate Go module boundary.
// It prevents root-level `go test ./...` from traversing JavaScript dependencies
// such as node_modules, while keeping backend Go packages under the root module.
module github.com/kongken/urlo/front

go 1.26.2
