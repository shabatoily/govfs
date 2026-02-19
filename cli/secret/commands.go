package secret

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

func RegisterCommands(target *cobra.Command) {
	secretCmd := NewSecretCmd()
	secretCmd.AddCommand(NewHashCmd(), NewCompareCmd())
	target.AddCommand(secretCmd)
}

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
		RunE: func(_ *cobra.Command, args []string) error {
			hashed, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			fmt.Println(string(hashed))
			return nil
		},
	}
}

func NewCompareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <hashed> <secret>",
		Short: "Compare a secret with a hashed secret",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			err := bcrypt.CompareHashAndPassword([]byte(args[1]), []byte(args[0]))
			if err != nil {
				return err
			}
			fmt.Println("Password matches")
			return nil
		},
	}
}
