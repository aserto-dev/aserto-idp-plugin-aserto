package srv

import (
	"context"
	"io"
	"log"

	aserto "github.com/aserto-dev/aserto-go/client"
	"github.com/aserto-dev/aserto-go/client/grpc"
	api "github.com/aserto-dev/go-grpc/aserto/api/v1"
	dir "github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1"
	"github.com/aserto-dev/idp-plugin-sdk/plugin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate mockgen -destination=mock_directory.go -package=srv github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1 DirectoryClient,Directory_LoadUsersClient

const (
	pageSize = int32(100)
)

type AsertoPlugin struct {
	Config          *AsertoConfig
	dirClient       dir.DirectoryClient
	ctx             context.Context
	token           string
	lastPage        bool
	loadUsersStream dir.Directory_LoadUsersClient
	sendCount       int32
	op              plugin.OperationType
}

func NewAuth0Plugin() *AsertoPlugin {
	return &AsertoPlugin{
		Config: &AsertoConfig{},
	}
}

func (s *AsertoPlugin) GetConfig() plugin.PluginConfig {
	return &AsertoConfig{}
}

func (s *AsertoPlugin) Open(cfg plugin.PluginConfig, operation plugin.OperationType) error {
	config, ok := cfg.(*AsertoConfig)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "invalid config")
	}
	s.Config = config

	s.ctx = context.Background()

	client, err := grpc.New(
		s.ctx,
		aserto.WithAddr(config.Authorizer),
		aserto.WithAPIKeyAuth(config.ApiKey),
		aserto.WithTenantID(config.Tenant),
	)
	if err != nil {
		log.Fatalf("Failed to create authorizer connection: %s", err)
	}

	s.dirClient = client.Directory
	s.lastPage = false
	switch operation {
	case plugin.OperationTypeWrite, plugin.OperationTypeDelete:
		{
			s.loadUsersStream, err = s.dirClient.LoadUsers(s.ctx)
			if err != nil {
				return err
			}
		}
	}

	s.sendCount = 0
	s.op = operation

	return nil
}

func (s *AsertoPlugin) Read() ([]*api.User, error) {
	if s.lastPage {
		return nil, io.EOF
	}
	resp, err := s.dirClient.ListUsers(s.ctx, &dir.ListUsersRequest{
		Page: &api.PaginationRequest{
			Size:  pageSize,
			Token: s.token,
		},
		Base: false,
	})
	if err != nil {
		return nil, err
	}

	if resp.Page.NextToken == "" {
		s.lastPage = true
	}

	s.token = resp.Page.NextToken

	return resp.Results, nil
}

func (s *AsertoPlugin) Write(user *api.User) error {

	req := &dir.LoadUsersRequest{
		Data: &dir.LoadUsersRequest_User{
			User: user,
		},
	}

	if err := s.loadUsersStream.Send(req); err != nil {
		return status.Errorf(codes.Internal, "stream send: %s", err.Error())
	}
	s.sendCount++

	return nil
}

func (s *AsertoPlugin) Delete(userId string) error {
	req := &dir.GetUserRequest{
		Id: userId,
	}

	resp, err := s.dirClient.GetUser(s.ctx, req)
	if err != nil {
		return status.Errorf(codes.Internal, "get user: %s", err.Error())
	}

	user := resp.GetResult()
	if user != nil {
		user.Deleted = true
		req := &dir.LoadUsersRequest{
			Data: &dir.LoadUsersRequest_User{
				User: user,
			},
		}

		if err := s.loadUsersStream.Send(req); err != nil {
			return status.Errorf(codes.Internal, "stream send: %s", err.Error())
		}
		s.sendCount++
	} else {
		return status.Errorf(codes.NotFound, "user %s not found", userId)
	}

	// req := &dir.DeleteUserRequest{
	// 	Id: userId,
	// }

	// if _, err := s.dirClient.DeleteUser(s.ctx, req); err != nil {
	// 	return errors.Wrapf(err, "delete %s", userId)
	// }

	return nil
}

func (s *AsertoPlugin) Close() (*plugin.Stats, error) {
	switch s.op {
	case plugin.OperationTypeWrite, plugin.OperationTypeDelete:
		{
			res, err := s.loadUsersStream.CloseAndRecv()
			if err != nil {
				return nil, status.Errorf(codes.Internal, "stream close: %s", err.Error())
			}

			if res != nil {
				return &plugin.Stats{
					Received: res.Received,
					Created:  res.Created,
					Updated:  res.Updated,
					Deleted:  res.Deleted,
					Errors:   res.Errors,
				}, nil
			}
		}
	}

	return nil, nil
}
