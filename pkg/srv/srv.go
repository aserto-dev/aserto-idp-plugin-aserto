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
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
	splitExtensions bool
}

func NewAuth0Plugin() *AsertoPlugin {
	return &AsertoPlugin{
		Config: &AsertoConfig{},
	}
}

func (s *AsertoPlugin) GetConfig() plugin.PluginConfig {
	return &AsertoConfig{}
}

func (s *AsertoPlugin) GetVersion() (string, string, string) {
	return GetVersion()
}

func (s *AsertoPlugin) Open(cfg plugin.PluginConfig, operation plugin.OperationType) error {
	config, ok := cfg.(*AsertoConfig)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "invalid config")
	}
	s.Config = config

	s.ctx = context.Background()

	var client *grpc.Client
	var err error
	if config.Insecure {
		client, err = grpc.New(
			s.ctx,
			aserto.WithAddr(config.Authorizer),
			aserto.WithAPIKeyAuth(config.ApiKey),
			aserto.WithTenantID(config.Tenant),
			aserto.WithInsecure(),
		)

	} else {
		client, err = grpc.New(
			s.ctx,
			aserto.WithAddr(config.Authorizer),
			aserto.WithAPIKeyAuth(config.ApiKey),
			aserto.WithTenantID(config.Tenant),
		)
	}

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
	s.splitExtensions = config.SplitExtensions

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

	var reqExt *dir.LoadUsersRequest
	if s.splitExtensions {
		clonedAttributes := proto.Clone(user.Attributes)
		user.Attributes = &api.AttrSet{}

		clonedApplications := make(map[string]*api.AttrSet)
		for k, v := range user.Applications {
			cloneAttrSet := proto.Clone(v)
			clonedApplications[k] = cloneAttrSet.(*api.AttrSet)
		}
		user.Applications = make(map[string]*api.AttrSet)

		var pid string

		for key, value := range user.Identities {
			if value.Kind == api.IdentityKind_IDENTITY_KIND_PID {
				pid = key
				break
			}
		}

		if pid == "" {
			return status.Errorf(codes.Internal, "couldn't find PID identity for user: %s", user.DisplayName)
		}

		reqExt = &dir.LoadUsersRequest{
			Data: &dir.LoadUsersRequest_UserExt{
				UserExt: &api.UserExt{
					Id:           pid,
					Attributes:   clonedAttributes.(*api.AttrSet),
					Applications: clonedApplications,
				},
			},
		}
	}

	req := &dir.LoadUsersRequest{
		Data: &dir.LoadUsersRequest_User{
			User: user,
		},
	}

	if err := s.loadUsersStream.Send(req); err != nil {
		return status.Errorf(codes.Internal, "stream send: %s", err.Error())
	}

	if reqExt != nil {
		if err := s.loadUsersStream.Send(reqExt); err != nil {
			return status.Errorf(codes.Internal, "stream send extension: %s", err.Error())
		}
	}

	s.sendCount++

	return nil
}

func (s *AsertoPlugin) Delete(userId string) error {
	var deleteUsers []*api.User
	if isValidUUID(userId) {
		req := &dir.GetUserRequest{
			Id: userId,
		}

		resp, err := s.dirClient.GetUser(s.ctx, req)
		if err != nil {
			return status.Errorf(codes.Internal, "get user: %s", err.Error())
		}

		user := resp.GetResult()
		if user == nil {
			return status.Errorf(codes.NotFound, "user %s not found", userId)
		}

		deleteUsers = append(deleteUsers, user)
	} else {
		var allUsers []*api.User
		for {
			u, err := s.Read()
			if err == io.EOF {
				break
			}
			allUsers = append(allUsers, u...)
		}

		for _, u := range allUsers {
			userJson, err := protojson.Marshal(u)
			if err != nil {
				return status.Errorf(codes.Internal, "unmarshal user: %s", err.Error())
			}
			userStr := string(userJson)
			result := gjson.Get("["+userStr+"]", userId)

			if result.Exists() {
				deleteUsers = append(deleteUsers, u)
			}
		}
	}

	for _, user := range deleteUsers {
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

func isValidUUID(u string) bool {
	_, err := uuid.Parse(u)
	return err == nil
}
