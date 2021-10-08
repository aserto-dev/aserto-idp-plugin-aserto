package srv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/aserto-dev/aserto-idp-plugin-aserto/pkg/grpcc"
	"github.com/aserto-dev/aserto-idp-plugin-aserto/pkg/grpcc/authorizer"
	api "github.com/aserto-dev/go-grpc/aserto/api/v1"
	dir "github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1"
)

// values set by linker using ldflag -X
var (
	ver    string // nolint:gochecknoglobals // set by linker
	date   string // nolint:gochecknoglobals // set by linker
	commit string // nolint:gochecknoglobals // set by linker
)

func GetVersion() (string, string, string) {
	return ver, date, commit
}

type AsertoConfig struct {
	Authorizer string `description:"Aserto authorizer endpoint" kind:"attribute" mode:"normal" readonly:"false"`
	Tenant     string `description:"Aserto Tenant ID" kind:"attribute" mode:"normal" readonly:"false"`
	ApiKey     string `description:"Aserto API Key" kind:"attribute" mode:"normal" readonly:"false"`
	IncludeExt bool   `description:"Include user extensions" kind:"attribute" mode:"normal" readonly:"false"`
}

func (c *AsertoConfig) Validate() error {

	if c.Authorizer == "" {
		return status.Error(codes.InvalidArgument, "no authorizer was provided")
	}

	if c.ApiKey == "" {
		return status.Error(codes.InvalidArgument, "no api key was provided")
	}

	if c.Tenant == "" {
		return status.Error(codes.InvalidArgument, "no tenant was provided")
	}

	ctx := context.Background()
	conn, err := authorizer.Connection(
		ctx,
		c.Authorizer,
		grpcc.NewAPIKeyAuth(c.ApiKey),
	)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to create authorizar connection %s", err.Error())
	}

	ctx = grpcc.SetTenantContext(ctx, c.Tenant)
	dirClient := conn.DirectoryClient()

	_, err = dirClient.ListUsers(ctx, &dir.ListUsersRequest{
		Page: &api.PaginationRequest{
			Size:  1,
			Token: "",
		},
		Base: !c.IncludeExt,
	})

	if err != nil {
		return status.Errorf(codes.Internal, "failed to get one user: %s", err.Error())
	}
	return nil
}

func (c *AsertoConfig) Description() string {
	return "Aserto plugin"
}
