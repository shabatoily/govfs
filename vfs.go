package vfs

import (
	"errors"
	"io"
	"iter"
	"time"

	"github.com/google/uuid"
)

const (
	Root = "/"

	DefaultFileMode = 0644
	DefaultDirMode  = 0755
)

var (
	ErrNotFound         = errors.New("no such file or directory")
	ErrAlreadyExists    = errors.New("file exists")
	ErrNotDir           = errors.New("not a directory")
	ErrInvalidPath      = errors.New("invalid path")
	ErrNotSupported     = errors.New("not supported")
	ErrNotSupportedSeek = errors.New("seek not supported")
)

type Meta struct {
	ID        uuid.UUID `json:"id"`
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	Extension string    `json:"extension"`
	Size      int64     `json:"size"`
	IsDir     bool      `json:"isDir"`
	Modified  time.Time `json:"modified"`
	Comments  string    `json:"comments"`
}

//nolint:gocyclo // switch case is not long
func (m *Meta) MIME() string {
	switch m.Extension {
	case "md":
		return "text/markdown"
	case "txt":
		return "text/plain"
	case "json":
		return "application/json"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "svg":
		return "image/svg+xml"
	case "html":
		return "text/html"
	case "pdf":
		return "application/pdf"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "avi":
		return "video/x-msvideo"
	case "mov":
		return "video/quicktime"
	case "mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}

type File struct {
	Meta *Meta
	r    io.ReadCloser
}

func (f *File) Read(p []byte) (n int, err error) {
	if f.r == nil {
		return 0, io.EOF
	}
	return f.r.Read(p)
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	if s, ok := f.r.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, ErrNotSupportedSeek
}

func (f *File) Close() error {
	if f.r == nil {
		return nil
	}
	return f.r.Close()
}

func NewFile(meta *Meta, r io.ReadCloser) *File {
	return &File{
		Meta: meta,
		r:    r,
	}
}

type TreeNode struct {
	Meta     Meta        `json:"meta"`
	Children []*TreeNode `json:"children,omitempty"` // 디렉토리일 경우 하위 노드들
}

func (node *TreeNode) Walk() iter.Seq[*TreeNode] {
	return func(yield func(*TreeNode) bool) {
		if node == nil {
			return
		}
		traverseNodes(node, yield)
	}
}

// 헬퍼 함수 정의 (재귀 순회)
func traverseNodes(node *TreeNode, yield func(*TreeNode) bool) bool {
	if node == nil {
		return true
	}
	// 현재 노드를 yield (반복문 바디 실행)
	// yield가 false를 반환하면 순회 중단을 의미
	if !yield(node) {
		return false
	}
	// 자식 노드 순회
	for _, child := range node.Children {
		if !traverseNodes(child, yield) {
			return false
		}
	}
	return true
}

type VFS interface {
	List(path string) ([]Meta, error)
	Open(id uuid.UUID) (*File, error)
	Create(path string, r io.Reader) (Meta, error)
	Write(id uuid.UUID, r io.Reader) (Meta, error)
	WriteComments(id uuid.UUID, comment string) (Meta, error)
	Delete(id uuid.UUID) error
	Mkdir(path string) (Meta, error)
	Stat(id uuid.UUID) (Meta, error)
	StatByPath(p string) (Meta, error)
	Move(id uuid.UUID, dst string) (Meta, error)
	Copy(id uuid.UUID, dst string) (Meta, error)
	Close() error
	Backup(w io.Writer, since uint64) (uint64, error)
	Load(r io.Reader, maxPendingWrites int) error
	Tree(path string) (*TreeNode, error)
}
