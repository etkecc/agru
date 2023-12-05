# show help by default
default:
    @just --list --justfile {{ justfile() }}

# update go deps
update *flags:
    go get {{flags}} .
    go mod tidy
    go mod vendor

# run linter
lint:
    golangci-lint run ./...

# automatically fix liter issues
lintfix:
    golangci-lint run --fix ./...

# run unit tests
test:
    @go test -coverprofile=cover.out ./...
    @go tool cover -func=cover.out
    -@rm -f cover.out

# run app
run:
    @go run .

# build app
build:
    go build -v -o agru .
