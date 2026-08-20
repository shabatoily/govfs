package services

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	badgerdb "github.com/dgraph-io/badger/v4"
	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"github.com/shabatoily/govfs/internal/types"
	driverbadger "github.com/shabatoily/govfs/pkg/drivers/badger"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound        = errors.New("user not found")
	ErrAlreadyExists   = errors.New("username already exists")
	ErrInvalidRole     = errors.New("invalid role")
	ErrInvalidPassword = errors.New("invalid credentials")
	ErrLastAdmin       = errors.New("last active admin cannot be changed")
)

var (
	userPrefix     = []byte("user:")
	usernamePrefix = []byte("username:")
)

type User struct {
	ID           uuid.UUID  `json:"id"`
	Username     string     `json:"username"`
	PasswordHash []byte     `json:"passwordHash"`
	Role         types.Role `json:"role"`
	Disabled     bool       `json:"disabled"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type UserUpdate struct {
	Role     *types.Role
	Disabled *bool
	Password string
}

type UserStore struct {
	db *badgerdb.DB
}

func OpenUserStore(path string) (*UserStore, error) {
	cfg := driverbadger.Config{Path: filepath.Clean(path)}
	db, err := badgerdb.Open(cfg.Options())
	if err != nil {
		return nil, err
	}
	return &UserStore{db: db}, nil
}

func (s *UserStore) Close() error {
	return s.db.Close()
}

func (s *UserStore) Create(username, password string, role types.Role) (User, error) {
	username = normalizeUsername(username)
	if username == "" || password == "" {
		return User{}, ErrInvalidPassword
	}
	if !role.Valid() {
		return User{}, ErrInvalidRole
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	now := time.Now().UTC()
	user := User{ID: uuid.New(), Username: username, PasswordHash: hash, Role: role, CreatedAt: now, UpdatedAt: now}
	err = s.db.Update(func(txn *badgerdb.Txn) error {
		if _, getErr := txn.Get(usernameKey(username)); getErr == nil {
			return ErrAlreadyExists
		} else if !errors.Is(getErr, badgerdb.ErrKeyNotFound) {
			return getErr
		}
		data, marshalErr := json.Marshal(user)
		if marshalErr != nil {
			return marshalErr
		}
		if setErr := txn.Set(userKey(user.ID), data); setErr != nil {
			return setErr
		}
		return txn.Set(usernameKey(username), user.ID[:])
	})
	return user, err
}

func (s *UserStore) Authenticate(username, password string) (User, error) {
	user, err := s.ByUsername(username)
	if err != nil || user.Disabled || bcrypt.CompareHashAndPassword(user.PasswordHash, []byte(password)) != nil {
		return User{}, ErrInvalidPassword
	}
	return user, nil
}

func (s *UserStore) ByID(id uuid.UUID) (User, error) {
	var user User
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, getErr := txn.Get(userKey(id))
		if errors.Is(getErr, badgerdb.ErrKeyNotFound) {
			return ErrNotFound
		}
		if getErr != nil {
			return getErr
		}
		return item.Value(func(data []byte) error { return json.Unmarshal(data, &user) })
	})
	return user, err
}

func (s *UserStore) ByUsername(username string) (User, error) {
	var id uuid.UUID
	err := s.db.View(func(txn *badgerdb.Txn) error {
		item, getErr := txn.Get(usernameKey(normalizeUsername(username)))
		if errors.Is(getErr, badgerdb.ErrKeyNotFound) {
			return ErrNotFound
		}
		if getErr != nil {
			return getErr
		}
		return item.Value(func(data []byte) error {
			if len(data) != len(id) {
				return fmt.Errorf("invalid user id length: %d", len(data))
			}
			copy(id[:], data)
			return nil
		})
	})
	if err != nil {
		return User{}, err
	}
	return s.ByID(id)
}

func (s *UserStore) List() ([]User, error) {
	users := make([]User, 0)
	err := s.db.View(func(txn *badgerdb.Txn) error {
		it := txn.NewIterator(badgerdb.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(userPrefix); it.ValidForPrefix(userPrefix); it.Next() {
			if err := it.Item().Value(func(data []byte) error {
				var user User
				if unmarshalErr := json.Unmarshal(data, &user); unmarshalErr != nil {
					return unmarshalErr
				}
				users = append(users, user)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return users, err
}

func (s *UserStore) Update(id uuid.UUID, update UserUpdate) (User, error) {
	if update.Role != nil && !update.Role.Valid() {
		return User{}, ErrInvalidRole
	}
	var hash []byte
	var err error
	if update.Password != "" {
		hash, err = bcrypt.GenerateFromPassword([]byte(update.Password), bcrypt.DefaultCost)
		if err != nil {
			return User{}, err
		}
	}
	var user User
	err = s.db.Update(func(txn *badgerdb.Txn) error {
		item, getErr := txn.Get(userKey(id))
		if errors.Is(getErr, badgerdb.ErrKeyNotFound) {
			return ErrNotFound
		}
		if getErr != nil {
			return getErr
		}
		if valueErr := item.Value(func(data []byte) error { return json.Unmarshal(data, &user) }); valueErr != nil {
			return valueErr
		}
		disabled, role := user.Disabled, user.Role
		if update.Disabled != nil {
			disabled = *update.Disabled
		}
		if update.Role != nil {
			role = *update.Role
		}
		if user.Role == types.RoleAdmin && !user.Disabled && (role != types.RoleAdmin || disabled) {
			count, countErr := activeAdminCount(txn)
			if countErr != nil {
				return countErr
			}
			if count == 1 {
				return ErrLastAdmin
			}
		}
		user.Role, user.Disabled = role, disabled
		if hash != nil {
			user.PasswordHash = hash
		}
		user.UpdatedAt = time.Now().UTC()
		data, marshalErr := json.Marshal(user)
		if marshalErr != nil {
			return marshalErr
		}
		return txn.Set(userKey(id), data)
	})
	return user, err
}

func activeAdminCount(txn *badgerdb.Txn) (int, error) {
	count := 0
	it := txn.NewIterator(badgerdb.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(userPrefix); it.ValidForPrefix(userPrefix); it.Next() {
		if err := it.Item().Value(func(data []byte) error {
			var user User
			if err := json.Unmarshal(data, &user); err != nil {
				return err
			}
			if user.Role == types.RoleAdmin && !user.Disabled {
				count++
			}
			return nil
		}); err != nil {
			return 0, err
		}
	}
	return count, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func userKey(id uuid.UUID) []byte {
	return append(append([]byte(nil), userPrefix...), id[:]...)
}

func usernameKey(username string) []byte {
	return append(append([]byte(nil), usernamePrefix...), username...)
}
