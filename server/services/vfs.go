package services

import (
	"bytes"
	"io"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/meteormin/go-vfs"
	"github.com/meteormin/go-vfs/server/types"
)

type VfsService struct {
	vfs    vfs.VFS
	prefix string
}

func (s *VfsService) Prefix() string {
	return s.prefix
}

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

func (s *VfsService) Tree(path string) (*types.TreeNodeRes, error) {
	treeNodes, err := s.vfs.Tree(path)
	if err != nil {
		return nil, err
	}
	return mapTreeNodeRes(s.prefix, treeNodes), nil
}

func (s *VfsService) Read(id uuid.UUID) (*vfs.File, error) {
	file, err := s.vfs.Open(id)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s *VfsService) Stat(id uuid.UUID) (types.MetaRes, error) {
	meta, err := s.vfs.Stat(id)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Create(name string, file io.Reader) (types.MetaRes, error) {
	meta, err := s.vfs.Create(name, file)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Mkdir(name string) (types.MetaRes, error) {
	meta, err := s.vfs.Mkdir(name)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Write(id uuid.UUID, content *bytes.Buffer) (types.MetaRes, error) {
	meta, err := s.vfs.Write(id, content)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Move(id uuid.UUID, dst string) (types.MetaRes, error) {
	meta, err := s.vfs.Move(id, dst)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Copy(id uuid.UUID, dst string) (types.MetaRes, error) {
	meta, err := s.vfs.Copy(id, dst)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Delete(id uuid.UUID) error {
	err := s.vfs.Delete(id)
	if err != nil {
		return err
	}
	return nil
}

func (s *VfsService) WriteComments(id uuid.UUID, comment string) (types.MetaRes, error) {
	meta, err := s.vfs.WriteComments(id, comment)
	if err != nil {
		return types.MetaRes{}, err
	}
	url := parseURL(s.prefix, &meta)
	return types.MetaRes{Meta: meta, URL: url}, nil
}

func (s *VfsService) Backup(w io.Writer) error {
	_, err := s.vfs.Backup(w, 0)
	return err
}

func (s *VfsService) Restore(r io.Reader) error {
	return s.vfs.Load(r, 256)
}

func (s *VfsService) Rotate(key string) error {
	return s.vfs.Rotate([]byte(key))
}

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
