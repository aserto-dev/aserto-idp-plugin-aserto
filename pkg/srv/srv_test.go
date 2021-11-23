package srv

import (
	"errors"
	"io"
	"testing"
	"time"

	api "github.com/aserto-dev/go-grpc/aserto/api/v1"
	directory "github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1"
	"github.com/aserto-dev/idp-plugin-sdk/plugin"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func CreateListResp(token string, users []*api.User) *directory.ListUsersResponse {

	return &directory.ListUsersResponse{
		Results: users,
		Page: &api.PaginationResponse{
			NextToken: token,
		},
	}

}

func CreateTestApiUser(id, displayName, email, mobilePhone string) *api.User {
	user := api.User{
		Id:          id,
		DisplayName: displayName,
		Email:       email,
		Picture:     "",
		Identities:  make(map[string]*api.IdentitySource),
		Attributes: &api.AttrSet{
			Properties:  &structpb.Struct{Fields: make(map[string]*structpb.Value)},
			Roles:       []string{},
			Permissions: []string{},
		},
		Applications: make(map[string]*api.AttrSet),
		Metadata: &api.Metadata{
			CreatedAt: timestamppb.New(time.Now()),
			UpdatedAt: timestamppb.New(time.Now()),
		},
	}

	user.Identities[mobilePhone] = &api.IdentitySource{
		Kind:     api.IdentityKind_IDENTITY_KIND_PHONE,
		Provider: "auth0",
		Verified: true,
	}

	return &user
}

func TestConstructor(t *testing.T) {
	// Arrange
	assert := require.New(t)

	// Act
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeRead)

	// Assert
	assert.NotNil(p)
}

func TestReadFailToRetriveUsers(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeRead)
	p.lastPage = false
	p.sendCount = 0

	p.dirClient.(*MockDirectoryClient).EXPECT().ListUsers(p.ctx, gomock.Any()).Return(
		nil, errors.New("BOOM!"))

	users, err := p.Read()

	assert.NotNil(err)
	assert.Equal("BOOM!", err.Error(), "should return error")
	assert.Nil(users)
}

func TestReadOnePage(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeRead)
	p.lastPage = false
	p.sendCount = 0
	var users []*api.User

	users = append(users, CreateTestApiUser("1", "First Last", "test@unit.com", "0998976834"))

	p.dirClient.(*MockDirectoryClient).EXPECT().ListUsers(p.ctx, gomock.Any()).Return(
		CreateListResp("", users), nil)

	users, err := p.Read()

	assert.Nil(err)
	assert.Len(users, 1)

	users, err = p.Read()
	assert.NotNil(err)
	assert.Equal(io.EOF, err, "read() should return EOF")
	assert.Nil(users)
}

func TestReadMultiplePages(t *testing.T) {
	// Arrange
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeRead)
	p.lastPage = false
	p.sendCount = 0
	var users []*api.User

	users = append(users, CreateTestApiUser("1", "First Last", "test@unit.com", "0998976834"))

	p.dirClient.(*MockDirectoryClient).EXPECT().ListUsers(p.ctx, gomock.Any()).Return(CreateListResp("nextPage", users), nil)

	// Act
	users1, err := p.Read()
	assert.Nil(err)

	users = nil
	users = append(users, CreateTestApiUser("2", "First2 Last2", "test@unit.com", "0998976834"))
	p.dirClient.(*MockDirectoryClient).EXPECT().ListUsers(p.ctx, gomock.Any()).Return(CreateListResp("nextPage", users), nil)
	users2, err := p.Read()

	// Assert
	assert.Nil(err)
	assert.Len(users1, 1)
	assert.Len(users2, 1)
	assert.NotEqual(users1[0].Id, users2[0].Id)
}

func TestWriteFail(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0
	user := CreateTestApiUser("1", "First Last", "test@unit.com", "0998976834")

	p.loadUsersStream.(*MockDirectory_LoadUsersClient).EXPECT().Send(gomock.Any()).Return(errors.New("BOOM!"))

	err := p.Write(user)

	assert.NotNil(err)
	assert.Equal("rpc error: code = Internal desc = stream send: BOOM!", err.Error(), "should return error")
	assert.Equal(int32(0), p.sendCount)
}

func TestWrite(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0
	user := CreateTestApiUser("1", "First Last", "test@unit.com", "0998976834")

	p.loadUsersStream.(*MockDirectory_LoadUsersClient).EXPECT().Send(gomock.Any()).Return(nil)

	err := p.Write(user)

	assert.Nil(err)
	assert.Equal(int32(1), p.sendCount)
}

func TestClose(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 1

	p.loadUsersStream.(*MockDirectory_LoadUsersClient).EXPECT().CloseAndRecv().Return(&directory.LoadUsersResponse{Received: 1}, nil)

	res, err := p.Close()
	assert.Nil(err)
	assert.Equal(int32(1), res.Received)
}

func TestCloseWithStreamClose(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0

	p.loadUsersStream.(*MockDirectory_LoadUsersClient).EXPECT().CloseAndRecv().Return(nil, errors.New("BOOM!"))

	res, err := p.Close()
	assert.NotNil(err)
	assert.Nil(res)
	assert.Equal("rpc error: code = Internal desc = stream close: BOOM!", err.Error(), "should return error")
}

func TestDeleteFail(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0

	p.dirClient.(*MockDirectoryClient).EXPECT().GetUser(p.ctx, gomock.Any()).Return(
		nil, errors.New("BOOM!"))

	err := p.Delete("id")
	assert.NotNil(err)
	assert.Equal("rpc error: code = Internal desc = get user: BOOM!", err.Error())
}

func TestDeleteWithInexistingUser(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0

	p.dirClient.(*MockDirectoryClient).EXPECT().GetUser(p.ctx, gomock.Any()).Return(
		&directory.GetUserResponse{Result: nil}, nil)

	err := p.Delete("id")
	assert.NotNil(err)
	assert.Equal("rpc error: code = NotFound desc = user id not found", err.Error())
}

func TestDelete(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0
	user := CreateTestApiUser("1", "First Last", "test@unit.com", "0998976834")

	p.dirClient.(*MockDirectoryClient).EXPECT().GetUser(p.ctx, gomock.Any()).Return(
		&directory.GetUserResponse{Result: user}, nil)
	p.loadUsersStream.(*MockDirectory_LoadUsersClient).EXPECT().Send(gomock.Any()).Return(nil)

	err := p.Delete("1")
	assert.Nil(err)
}

func TestDeleteStreamFail(t *testing.T) {
	assert := require.New(t)
	p := NewTestAsertoPlugin(gomock.NewController(t), plugin.OperationTypeWrite)
	p.lastPage = false
	p.sendCount = 0
	user := CreateTestApiUser("1", "First Last", "test@unit.com", "0998976834")

	p.dirClient.(*MockDirectoryClient).EXPECT().GetUser(p.ctx, gomock.Any()).Return(
		&directory.GetUserResponse{Result: user}, nil)
	p.loadUsersStream.(*MockDirectory_LoadUsersClient).EXPECT().Send(gomock.Any()).Return(errors.New("BOOM!"))

	err := p.Delete("1")
	assert.NotNil(err)
	assert.Equal("rpc error: code = Internal desc = stream send: BOOM!", err.Error())
}
