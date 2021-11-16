//+build wireinject

package srv

import (
	"context"

	"github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1"
	"github.com/aserto-dev/idp-plugin-sdk/plugin"
	gomock "github.com/golang/mock/gomock"
	"github.com/google/wire"
)

func NewAsertoPlugin() *AsertoPlugin {
	wire.Build(
		wire.Struct(new(AsertoPlugin)),
	)

	return &AsertoPlugin{}
}

func NewTestAsertoPlugin(ctrl *gomock.Controller, op plugin.OperationType) *AsertoPlugin {
	wire.Build(
		wire.Struct(new(AsertoPlugin), "ctx", "dirClient", "loadUsersStream", "op"),
		context.Background,
		wire.Bind(new(directory.DirectoryClient), new(*MockDirectoryClient)),
		wire.Bind(new(directory.Directory_LoadUsersClient), new(*MockDirectory_LoadUsersClient)),
		NewMockDirectoryClient,
		NewMockDirectory_LoadUsersClient,
	)

	return &AsertoPlugin{}
}
