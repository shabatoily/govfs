// Package secret은 CLI에서 비밀번호 해싱 및 생성 등 보안 관련 명령을 제공합니다.
package secret

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

const (
	// Base64 RawURLEncoding 시 16자리를 생성하기 위한 소스 바이트 길이
	// 12 bytes * 8 bits / 6 bits per char = 16 chars
	secretLength = 12
)

// RegisterCommands는 `secret`과 관련된 모든 하위 명령을 등록합니다.
func RegisterCommands(target *cobra.Command) {
	secretCmd := NewSecretCmd()
	secretCmd.AddCommand(NewHashCmd(), NewCompareCmd(), NewGenerateHashCmd())
	target.AddCommand(secretCmd)
}

// NewSecretCmd는 `secret` 명령 그룹을 생성합니다.
func NewSecretCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "secret",
		Short: "Manage secrets",
	}
}

func NewHashCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hash <secret>",
		Short: "Hash a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hashed, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			cmd.Println(string(hashed))
			return nil
		},
	}
}

func NewCompareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <hashed> <secret>",
		Short: "Compare a secret with a hashed secret",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			err := bcrypt.CompareHashAndPassword([]byte(args[0]), []byte(args[1]))
			if err != nil {
				return err
			}
			cmd.Println("Password matches")
			return nil
		},
	}
}

func NewGenerateHashCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate a random hashed secret",
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret := make([]byte, secretLength)
			_, err := rand.Read(secret)
			if err != nil {
				return err
			}
			hashed, err := bcrypt.GenerateFromPassword(secret, bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			cmd.Printf("Raw: %s\n", base64.RawURLEncoding.EncodeToString(secret))
			cmd.Printf("Hashed: %s\n", string(hashed))
			return nil
		},
	}
}
