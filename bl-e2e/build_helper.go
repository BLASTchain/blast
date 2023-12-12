package op_e2e

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BuildOpProgramClient builds the `bl-program` client executable and returns the path to the resulting executable
func BuildOpProgramClient(t *testing.T) string {
	t.Log("Building bl-program-client")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "make", "bl-program-client")
	cmd.Dir = "../bl-program"
	cmd.Stdout = os.Stdout // for debugging
	cmd.Stderr = os.Stderr // for debugging
	require.NoError(t, cmd.Run(), "Failed to build bl-program-client")
	t.Log("Built bl-program-client successfully")
	return "../bl-program/bin/bl-program-client"
}
