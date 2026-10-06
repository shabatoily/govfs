// Package admin은 CLI 관리자 명령과 핸들러를 제공합니다.
package admin

import (
	"errors"
	"uuid"

	"github.com/shabatoily/govfs/internal/types"
	"github.com/spf13/cobra"
)

// RegisterCommands는 관리자 명령을 등록합니다.
func RegisterCommands(target *cobra.Command) {
	target.AddCommand(NewCommand())
}

// NewCommand는 관리자 명령을 기능별로 구성합니다.
func NewCommand() *cobra.Command {
	command := &cobra.Command{Use: "admin", Short: "Manage users and inspect server system state"}
	users := &cobra.Command{Use: "users", Short: "Manage users"}
	users.AddCommand(NewListUsersCommand(), NewUserStatusCommand(),
		NewCreateUserCommand(), NewUpdateUserCommand(), NewClearUserEventsCommand())
	system := &cobra.Command{Use: "system", Short: "Inspect system database"}
	system.AddCommand(NewSystemEntriesCommand())
	command.AddCommand(NewStatusCommand(), users, NewEventsCommand(), system)
	return command
}

// NewStatusCommand는 서버 상태 조회 명령을 생성합니다.
func NewStatusCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "status",
		Short: "Show server system status",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Status()
		},
	}
	return command
}

// NewListUsersCommand는 사용자 목록 조회 명령을 생성합니다.
func NewListUsersCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "list",
		Short: "List users",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.ListUsers()
		},
	}
	return command
}

// NewUserStatusCommand는 사용자 상태 조회 명령을 생성합니다.
func NewUserStatusCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "status <id>",
		Short: "Show user drive and connection status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return err
			}

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.UserStatus(id)
		},
	}
	return command
}

// NewCreateUserCommand는 사용자 생성 명령을 생성합니다.
func NewCreateUserCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "create <username>",
		Short: "Create a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			role, _ := cmd.Flags().GetString("role")
			if !types.Role(role).Valid() {
				return errors.New("invalid role")
			}
			password, _ := cmd.Flags().GetString("password")

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.CreateUser(types.CreateUserReq{Username: args[0], Password: password, Role: types.Role(role)})
		},
	}
	command.Flags().String("role", "user", "User role: admin or user")
	command.Flags().String("password", "", "Initial password")
	_ = command.MarkFlagRequired("password")
	return command
}

// NewUpdateUserCommand는 사용자 수정 명령을 생성합니다.
func NewUpdateUserCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "update <id>",
		Short: "Update user role, password or disabled state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return err
			}
			var req types.UpdateUserReq
			flags := cmd.Flags()
			if flags.Changed("role") {
				role, _ := flags.GetString("role")
				r := types.Role(role)
				if !r.Valid() {
					return errors.New("invalid role")
				}
				req.Role = &r
			}
			if flags.Changed("disabled") {
				disabled, _ := flags.GetBool("disabled")
				req.Disabled = &disabled
			}
			if flags.Changed("password") {
				req.Password, _ = flags.GetString("password")
				if req.Password == "" {
					return errors.New("password must not be empty")
				}
			}
			if !flags.Changed("role") && !flags.Changed("disabled") && !flags.Changed("password") {
				return errors.New("provide --role, --disabled or --password")
			}

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.UpdateUser(id, req)
		},
	}
	command.Flags().String("role", "", "User role: admin or user")
	command.Flags().String("password", "", "New password")
	command.Flags().Bool("disabled", false, "Disable user (use --disabled=false to enable)")
	return command
}

// NewClearUserEventsCommand는 사용자 이벤트 삭제 명령을 생성합니다.
func NewClearUserEventsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "clear-events <id>",
		Short: "Delete all events for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return err
			}

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.ClearUserEvents(id)
		},
	}
	return command
}

// NewEventsCommand는 감사 이벤트 조회 명령을 생성합니다.
func NewEventsCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "events",
		Short: "List audit events",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			page, size, err := pagination(cmd)
			if err != nil {
				return err
			}
			userID, _ := cmd.Flags().GetString("user-id")
			if userID != "" {
				if _, err := uuid.Parse(userID); err != nil {
					return err
				}
			}

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.Events(page, size, userID)
		},
	}
	command.Flags().String("user-id", "", "Filter by user UUID")
	addPaginationFlags(command)
	return command
}

// NewSystemEntriesCommand는 시스템 DB 조회 명령을 생성합니다.
func NewSystemEntriesCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "entries",
		Short: "List sanitized system database entries",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			page, size, err := pagination(cmd)
			if err != nil {
				return err
			}

			h, err := NewHandler(cmd)
			if err != nil {
				return err
			}
			return h.SystemEntries(page, size)
		},
	}
	addPaginationFlags(command)
	return command
}

func addPaginationFlags(cmd *cobra.Command) {
	cmd.Flags().Int("page", 1, "Page number")
	cmd.Flags().Int("page-size", 20, "Items per page (1-100)")
}

func pagination(cmd *cobra.Command) (int, int, error) {
	page, _ := cmd.Flags().GetInt("page")
	size, _ := cmd.Flags().GetInt("page-size")
	if page < 1 || size < 1 || size > 100 {
		return 0, 0, errors.New("page must be positive and page size must be between 1 and 100")
	}
	return page, size, nil
}
