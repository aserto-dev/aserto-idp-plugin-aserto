package srv

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/aserto-dev/aserto-idp-plugin-aserto/pkg/grpcc"
	"github.com/aserto-dev/aserto-idp-plugin-aserto/pkg/grpcc/authorizer"
	api "github.com/aserto-dev/go-grpc/aserto/api/v1"
	dir "github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1"
	"github.com/aserto-dev/idp-plugin-sdk/plugin"
	"github.com/pkg/errors"
)

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
		return errors.New("invalid config")
	}
	s.Config = config

	s.ctx = context.Background()

	conn, err := authorizer.Connection(
		s.ctx,
		config.Authorizer,
		grpcc.NewAPIKeyAuth(config.ApiKey),
	)
	if err != nil {
		log.Fatalf("Failed to create authorizer connection: %s", err)
	}

	s.ctx = grpcc.SetTenantContext(s.ctx, s.Config.Tenant)
	s.dirClient = conn.DirectoryClient()
	s.lastPage = false
	s.loadUsersStream, err = s.dirClient.LoadUsers(s.ctx)
	s.sendCount = 0
	s.op = operation
	if err != nil {
		return err
	}
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
		Base: !s.Config.IncludeExt,
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
	if !s.Config.IncludeExt {
		user.Attributes = &api.AttrSet{}
		user.Applications = make(map[string]*api.AttrSet)
	}

	req := &dir.LoadUsersRequest{
		Data: &dir.LoadUsersRequest_User{
			User: user,
		},
	}

	if err := s.loadUsersStream.Send(req); err != nil {
		return errors.Wrapf(err, "stream send %s", user.Id)
	}
	s.sendCount++

	return nil
}

func (s *AsertoPlugin) Delete(userId string) error {
	req := &dir.DeleteUserRequest{
		Id: userId,
	}

	if _, err := s.dirClient.DeleteUser(s.ctx, req); err != nil {
		return errors.Wrapf(err, "delete %s", userId)
	}

	return nil
}

func (s *AsertoPlugin) Close() error {
	switch s.op {
	case plugin.OperationTypeWrite:
		{
			res, err := s.loadUsersStream.CloseAndRecv()
			if err != nil {
				return errors.Wrapf(err, "stream.CloseAndRecv()")
			}

			if res != nil && res.Received != s.sendCount {
				return fmt.Errorf("send != received %d - %d", s.sendCount, res.Received)
			}
		}
	}

	return nil
}
