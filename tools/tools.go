//go:build tools

package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/goreleaser/goreleaser/v2"
	_ "github.com/ionos-cloud/mockserver-client-go/pkg/client"
	_ "github.com/samber/lo"
	_ "gotest.tools/gotestsum"
)
