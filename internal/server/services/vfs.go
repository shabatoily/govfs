// Package services는 서버의 핵심 비즈니스 로직을 제공합니다.
package services

import (
	"bytes"
	"io"
	"path/filepath"

	"github.com/google/uuid"
	vfs "github.com/meteormin/govfs"
	"github.com/meteormin/govfs/internal/types"
	"github.com/meteormin/govfs/pkg/drivers/badger"
)

// VfsService는 VFS 드라이버를 래핑하여 서버에 특화된 기능을 제공합니다.
type VfsService struct {
	vfs    vfs.VFS // 백엔드 VFS 드라이버
	prefix string  // 리소스 접근용 URL 프리픽스
}

// Prefix는 연동된 VFS 리소스의 기본 접근 URL 접두사를 반환합니다.
func (s *VfsService) Prefix() string {
	return s.prefix
}

// List는 특정 경로의 항목 목록을 조회하고 접근 URL을 포함한 결과를 반환합니다.
func (s *VfsService) List(path string) ([]types.MetaRes, error) {
	metas, err := s.vfs.List(path)
	if err != nil {
		return nil, err
	}

	metaRes := make([]types.MetaRes, 0)
	for _, m := range metas {
		url := parseURL(s.prefix, &m)
		metaRes = append(metaRes, types.MetaRes{Meta: m, URL: url})
	}

	return metaRes, nil
}

// Tree는 특정 경로 이하의 구조를 트리 형태로 반환합니다.
func (s *VfsService) Tree(path string) (*types.TreeNodeRes, error) {
	treeNodes, err := s.vfs.Tree(path)
	if err != nil {
		return nil, err
	}
	return mapTreeNodeRes(s.prefix, treeNodes), nil
}

// Read는 지정된 ID의 파일 핸들을 엽니다.
func (s *VfsService) Read(id uuid.UUID) (*vfs.File, error) {
	file, err := s.vfs.Open(id)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// Stat은 지정된 ID의 메타데이터를 조회합니다.
func (s *VfsService) Stat(id uuid.UUID) (types.MetaRes, error) {
	meta, err := s.vfs.Stat(id)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Create은 새로운 파일을 생성합니다.
func (s *VfsService) Create(name string, file io.Reader) (types.MetaRes, error) {
	meta, err := s.vfs.Create(name, file)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Mkdir은 새로운 디렉토리를 생성합니다.
func (s *VfsService) Mkdir(name string) (types.MetaRes, error) {
	meta, err := s.vfs.Mkdir(name)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Write는 파일 내용을 업데이트합니다.
func (s *VfsService) Write(id uuid.UUID, content *bytes.Buffer) (types.MetaRes, error) {
	meta, err := s.vfs.Write(id, content)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Move는 파일 또는 디렉토리를 이동합니다.
func (s *VfsService) Move(id uuid.UUID, dst string) (types.MetaRes, error) {
	meta, err := s.vfs.Move(id, dst)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Copy는 파일 또는 디렉토리를 복사합니다.
func (s *VfsService) Copy(id uuid.UUID, dst string) (types.MetaRes, error) {
	meta, err := s.vfs.Copy(id, dst)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Delete는 파일 또는 디렉토리를 삭제합니다.
func (s *VfsService) Delete(id uuid.UUID) error {
	err := s.vfs.Delete(id)
	if err != nil {
		return err
	}
	return nil
}

// WriteComments는 항목에 대한 설명을 업데이트합니다.
func (s *VfsService) WriteComments(id uuid.UUID, comment string) (types.MetaRes, error) {
	meta, err := s.vfs.WriteComments(id, comment)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

// Backup은 전체 VFS 데이터를 백업 스트림으로 출력합니다.
func (s *VfsService) Backup(w io.Writer) error {
	_, err := s.vfs.Backup(w, 0)
	return err
}

// Restore는 백업 스트림으로부터 데이터를 복구합니다.
func (s *VfsService) Restore(r io.Reader) error {
	return s.vfs.Load(r, 256)
}

// Rotate는 (지원되는 경우) 암호화 키를 교체합니다.
func (s *VfsService) Rotate(key string) error {
	badgerVFS, ok := s.vfs.(*badger.BadgerVFS)
	if !ok {
		return vfs.ErrNotSupported
	}
	return badgerVFS.Rotate([]byte(key))
}

// NewVfsService는 새로운 VfsService 인스턴스를 생성합니다.
func NewVfsService(fs vfs.VFS, prefix string) *VfsService {
	return &VfsService{prefix: prefix, vfs: fs}
}

func parseURL(prefix string, m *vfs.Meta) string {
	if m.IsDir {
		return prefix + "?q=" + m.Path
	}
	return filepath.Join(prefix, m.ID.String())
}

func mapTreeNodeRes(prefix string, node *vfs.TreeNode) *types.TreeNodeRes {
	if node == nil {
		return nil
	}

	// 1. 하위 노드(Children) 재귀 변환
	var childrenRes []*types.TreeNodeRes
	if len(node.Children) > 0 {
		childrenRes = make([]*types.TreeNodeRes, len(node.Children))
		for i, child := range node.Children {
			childrenRes[i] = mapTreeNodeRes(prefix, child) // 자기 자신을 다시 호출
		}
	}

	// 2. 결과 구조체 조립
	url := parseURL(prefix, &node.Meta)
	return &types.TreeNodeRes{
		Meta:     types.MetaRes{Meta: node.Meta, URL: url},
		Children: childrenRes,
	}
}
