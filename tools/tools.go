//go:build tools

package tools

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/google/go-licenses"
	_ "github.com/goreleaser/goreleaser/v2"
	_ "gotest.tools/gotestsum"
)
