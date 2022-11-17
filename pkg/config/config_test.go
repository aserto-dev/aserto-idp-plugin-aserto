package config_test

import (
	"testing"

	"github.com/aserto-dev/aserto-idp-plugin-aserto/pkg/config"
	"github.com/aserto-dev/idp-plugin-sdk/plugin"
	"github.com/stretchr/testify/require"
)

func TestValidateWithEmptyAuthorizer(t *testing.T) {
	assert := require.New(t)
	cfg := config.AsertoConfig{
		Authorizer: "",
		APIKey:     "APIKey",
		Tenant:     "tenantID",
	}
	err := cfg.Validate(plugin.OperationTypeRead)

	assert.NotNil(err)
	assert.Equal("rpc error: code = InvalidArgument desc = no authorizer was provided", err.Error())
}

func TestValidateWithEmptyAPIKey(t *testing.T) {
	assert := require.New(t)
	cfg := config.AsertoConfig{
		Authorizer: "Auth",
		APIKey:     "",
		Tenant:     "tenantID",
	}

	err := cfg.Validate(plugin.OperationTypeRead)

	assert.NotNil(t, err)
	assert.Equal("rpc error: code = InvalidArgument desc = no api key was provided", err.Error())
}

func TestValidateWithEmptyTenantID(t *testing.T) {
	assert := require.New(t)
	cfg := config.AsertoConfig{
		Authorizer: "Auth",
		APIKey:     "APIKey",
		Tenant:     "",
	}

	err := cfg.Validate(plugin.OperationTypeRead)

	assert.NotNil(t, err)
	assert.Equal("rpc error: code = InvalidArgument desc = no tenant was provided", err.Error())
}

func TestValidateWithInvalidCredentials(t *testing.T) {
	assert := require.New(t)
	cfg := config.AsertoConfig{
		Authorizer: "Auth",
		APIKey:     "APIKey",
		Tenant:     "Tenant",
	}

	err := cfg.Validate(plugin.OperationTypeRead)

	assert.NotNil(t, err)
	assert.Equal("rpc error: code = Internal desc = failed to create authorizer connection create grpc client failed: context deadline exceeded", err.Error())
}

func TestDecription(t *testing.T) {
	assert := require.New(t)
	cfg := config.AsertoConfig{
		Authorizer: "Auth",
		APIKey:     "APIKey",
		Tenant:     "tenantID",
	}

	description := cfg.Description()

	assert.Equal("Aserto plugin", description, "should return the description of the plugin")
}
