package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsImageGenerationGroupPermissionError(t *testing.T) {
	require.True(t, IsImageGenerationGroupPermissionError(403, "Image generation is not enabled for this group"))
	require.False(t, IsImageGenerationGroupPermissionError(403, "temporary forbidden"))
	require.False(t, IsImageGenerationGroupPermissionError(429, "Image generation is not enabled for this group"))
}
