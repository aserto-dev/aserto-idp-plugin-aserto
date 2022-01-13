package srv

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aserto "github.com/aserto-dev/aserto-go/client"
	"github.com/aserto-dev/aserto-go/client/authorizer"
	api "github.com/aserto-dev/go-grpc/aserto/api/v1"
	dir "github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1"
	"github.com/aserto-dev/idp-plugin-sdk/plugin"
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
	Authorizer      string `description:"Aserto authorizer endpoint" kind:"attribute" mode:"normal" readonly:"false" name:"authorizer"`
	Tenant          string `description:"Aserto Tenant ID" kind:"attribute" mode:"normal" readonly:"false" name:"tenant"`
	ApiKey          string `description:"Aserto API Key" kind:"attribute" mode:"normal" readonly:"false" name:"api_key"`
	SplitExtensions bool   `description:"Split user and extensions" kind:"attribute" mode:"normal" readonly:"false" name:"split_extensions"`
	Insecure        bool   `description:"Disable TLS verification if true" kind:"attribute" mode:"normal" readonly:"false" name:"insecure"`
}

func (c *AsertoConfig) Validate(operation plugin.OperationType) error {

	if c.Authorizer == "" {
		return status.Error(codes.InvalidArgument, "no authorizer was provided")
	}

	if c.ApiKey == "" && !c.Insecure {
		return status.Error(codes.InvalidArgument, "no api key was provided")
	}

	if c.Tenant == "" {
		return status.Error(codes.InvalidArgument, "no tenant was provided")
	}

	ctx := context.Background()
	var client *authorizer.Client
	var err error

	if c.Insecure {
		client, err = authorizer.New(
			ctx,
			aserto.WithAddr(c.Authorizer),
			aserto.WithTenantID(c.Tenant),
			aserto.WithInsecure(c.Insecure),
		)
	} else {
		client, err = authorizer.New(
			ctx,
			aserto.WithAddr(c.Authorizer),
			aserto.WithAPIKeyAuth(c.ApiKey),
			aserto.WithTenantID(c.Tenant),
		)
	}

	if err != nil {
		return status.Errorf(codes.Internal, "failed to create authorizer connection %s", err.Error())
	}

	_, err = client.Directory.ListUsers(ctx, &dir.ListUsersRequest{
		Page: &api.PaginationRequest{
			Size:  1,
			Token: "",
		},
		Base: false,
	})

	if err != nil {
		return status.Errorf(codes.Internal, "failed to get one user: %s", err.Error())
	}
	return nil
}

func (c *AsertoConfig) Description() string {
	return "Aserto plugin"
}
