package vfs

import (
	"errors"
	"io"
	"iter"
	"time"

	"github.com/google/uuid"
)

// 기본 디렉터리 및 권한 설정 상수입니다.
const (
	// Root는 최상위 디렉터리 경로를 나타냅니다.
	Root = "/"

	// DefaultFileMode는 파일 생성 시 사용되는 기본 권한(0644)입니다.
	DefaultFileMode = 0644
	// DefaultDirMode는 디렉터리 생성 시 사용되는 기본 권한(0755)입니다.
	DefaultDirMode  = 0755
)

// VFS 작업 중 발생할 수 있는 주요 에러들입니다.
var (
	ErrNotFound         = errors.New("no such file or directory")
	ErrAlreadyExists    = errors.New("file exists")
	ErrNotDir           = errors.New("not a directory")
	ErrInvalidPath      = errors.New("invalid path")
	ErrNotSupported     = errors.New("not supported")
	ErrNotSupportedSeek = errors.New("seek not supported")
)

// Meta는 가상 파일 시스템(VFS) 내 파일 또는 디렉터리의 메타데이터 정보를 담고 있습니다.
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

// MIME은 파일 확장자를 기반으로 적절한 MIME 타입을 반환합니다.
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

// File은 열린 파일에 대한 메타데이터와 데이터를 읽을 수 있는 인터페이스를 제공하는 구조체입니다.
type File struct {
	Meta *Meta
	r    io.ReadCloser
}

// Read는 파일에서 데이터를 읽어 p에 저장합니다.
func (f *File) Read(p []byte) (n int, err error) {
	if f.r == nil {
		return 0, io.EOF
	}
	return f.r.Read(p)
}

// Seek은 파일의 읽기 포인터 위치를 이동시킵니다. io.Seeker를 구현합니다.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if s, ok := f.r.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, ErrNotSupportedSeek
}

// Close는 열려있는 파일을 닫고 관련 리소스를 해제합니다.
func (f *File) Close() error {
	if f.r == nil {
		return nil
	}
	return f.r.Close()
}

// NewFile은 주어진 메타데이터와 ReadCloser를 사용하여 새로운 File 구조체를 생성합니다.
func NewFile(meta *Meta, r io.ReadCloser) *File {
	return &File{
		Meta: meta,
		r:    r,
	}
}

// TreeNode는 트리 구조 형태로 디렉터리와 파일을 표현할 때 사용하는 구조체입니다.
type TreeNode struct {
	Meta     Meta        `json:"meta"`
	Children []*TreeNode `json:"children,omitempty"` // 디렉터리일 경우 하위 노드들을 포함합니다.
}

// Walk는 현재 노드부터 시작하여 하위 트리의 모든 노드를 순회할 수 있는 이터레이터를 반환합니다.
func (node *TreeNode) Walk() iter.Seq[*TreeNode] {
	return func(yield func(*TreeNode) bool) {
		if node == nil {
			return
		}
		traverseNodes(node, yield)
	}
}

// traverseNodes는 트리 노드를 재귀적으로 순회하는 내부 헬퍼 함수입니다.
// yield 함수가 false를 반환하면 전체 순회를 즉시 중단합니다.
func traverseNodes(node *TreeNode, yield func(*TreeNode) bool) bool {
	if node == nil {
		return true
	}
	// 현재 노드를 yield에 전달하여 로직을 실행합니다.
	// yield가 false를 반환하면 순회 중단을 의미합니다.
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

// VFS는 시스템이 제공하는 범용 가상 파일 시스템(Virtual File System) 인터페이스입니다.
// 파일 및 디렉터리 조회, 생성, 수정, 삭제 등 전반적인 파일 시스템 작업을 정의합니다.
type VFS interface {
	// List는 특정 경로에 존재하는 자식 메타데이터 목록을 반환합니다.
	List(path string) ([]Meta, error)
	// Open은 고유 ID를 기반으로 파일을 엽니다.
	Open(id uuid.UUID) (*File, error)
	// Create는 주어진 경로에 새 파일을 생성합니다.
	Create(path string, r io.Reader) (Meta, error)
	// Write는 고유 ID를 가진 파일의 내용을 덮어씁니다.
	Write(id uuid.UUID, r io.Reader) (Meta, error)
	// WriteComments는 파일 또는 디렉터리의 코멘트 속성을 수정합니다.
	WriteComments(id uuid.UUID, comment string) (Meta, error)
	// Delete는 파일을 삭제합니다.
	Delete(id uuid.UUID) error
	// Mkdir은 새로운 디렉터리를 생성합니다.
	Mkdir(path string) (Meta, error)
	// Stat은 고유 ID를 통해 파일 또는 디렉터리의 메타데이터를 조회합니다.
	Stat(id uuid.UUID) (Meta, error)
	// StatByPath는 경로 문자열을 통해 파일 또는 디렉터리의 메타데이터를 조회합니다.
	StatByPath(p string) (Meta, error)
	// Move는 파일 또는 디렉터리를 새로운 경로로 이동시킵니다.
	Move(id uuid.UUID, dst string) (Meta, error)
	// Copy는 파일 또는 디렉터리를 다른 경로로 복사합니다.
	Copy(id uuid.UUID, dst string) (Meta, error)
	// Close는 시스템을 정상적으로 종료하고 점유 중인 리소스를 반환합니다.
	Close() error
	// Backup은 지정된 시점(since) 이후에 변경된 데이터들을 스트림(w)으로 백업합니다.
	Backup(w io.Writer, since uint64) (uint64, error)
	// Load는 백업된 데이터 스트림(r)으로부터 파일 시스템 상태를 복원합니다.
	Load(r io.Reader, maxPendingWrites int) error
	// Tree는 지정된 경로를 루트로 하는 트리 구조 데이터를 반환합니다.
	Tree(path string) (*TreeNode, error)
}
