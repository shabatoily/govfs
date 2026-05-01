package badger

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_randomSecretKey(t *testing.T) {
	key, err := randomSecretKey(32)
	require.NotNil(t, key)
	require.NoError(t, err)
}

func Test_GenerateEncryptionKey(t *testing.T) {
	tempFile := ".secret.tmp"
	t.Cleanup(func() {
		err := os.Remove(tempFile)
		if err != nil {
			t.Error(err)
		}
	})

	key, err := GenerateEncryptionKey(tempFile, 32)
	require.NotNil(t, key)
	require.NoError(t, err)
}
