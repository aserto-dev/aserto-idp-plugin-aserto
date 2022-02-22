package mocks

//go:generate mockgen -destination=mock_directory.go -package=mocks github.com/aserto-dev/go-grpc/aserto/authorizer/directory/v1 DirectoryClient,Directory_LoadUsersClient
