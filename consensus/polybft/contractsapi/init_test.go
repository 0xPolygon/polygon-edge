package contractsapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactNotEmpty(t *testing.T) {
	require.NotEmpty(t, CheckpointManager.Bytecode)
	require.NotEmpty(t, CheckpointManager.DeployedBytecode)
	require.NotEmpty(t, CheckpointManager.Abi)

	require.NotEmpty(t, ExitHelper.Bytecode)
	require.NotEmpty(t, ExitHelper.DeployedBytecode)
	require.NotEmpty(t, ExitHelper.Abi)

	require.NotEmpty(t, L2StateSender.Bytecode)
	require.NotEmpty(t, L2StateSender.DeployedBytecode)
	require.NotEmpty(t, L2StateSender.Abi)

	require.NotEmpty(t, BLS.Bytecode)
	require.NotEmpty(t, BLS.DeployedBytecode)
	require.NotEmpty(t, BLS.Abi)

	require.NotEmpty(t, BLS256.Bytecode)
	require.NotEmpty(t, BLS256.DeployedBytecode)
	require.NotEmpty(t, BLS256.Abi)
}
