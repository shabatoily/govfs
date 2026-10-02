// Package services는 서버의 핵심 비즈니스 로직을 제공합니다.
package services

import (
	"io"
	"net/url"
	"path"
	"strings"
	"uuid"

	vfs "github.com/shabatoily/govfs"
	"github.com/shabatoily/govfs/internal/types"
)

// VfsService는 VFS 드라이버를 래핑하여 서버에 특화된 기능을 제공합니다.
type VfsService struct {
	vfs    vfs.VFS // 백엔드 VFS 드라이버
	prefix string  // 리소스 접근용 URL 프리픽스
}

// List는 특정 경로의 항목 목록을 조회하고 접근 URL을 포함한 결과를 반환합니다.
func (s *VfsService) List(directoryPath string) ([]types.MetaRes, error) {
	metas, err := s.vfs.List(directoryPath)
	if err != nil {
		return nil, err
	}

	metaRes := make([]types.MetaRes, len(metas))
	for i, m := range metas {
		metaRes[i] = metaResponse(s.prefix, m)
	}

	return metaRes, nil
}

// Tree는 특정 경로 이하의 구조를 트리 형태로 반환합니다.
func (s *VfsService) Tree(directoryPath string) (*types.TreeNodeRes, error) {
	treeNodes, err := s.vfs.Tree(directoryPath)
	if err != nil {
		return nil, err
	}
	return mapTreeNodeRes(s.prefix, treeNodes), nil
}

// Search는 전체 VFS에서 이름에 검색어가 포함된 파일과 디렉터리를 반환합니다.
func (s *VfsService) Search(query string) ([]types.MetaRes, error) {
	tree, err := s.vfs.Tree(vfs.Root)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	results := make([]types.MetaRes, 0)
	for node := range tree.Walk() {
		if node.Meta.Path == vfs.Root || !strings.Contains(strings.ToLower(node.Meta.Name), query) {
			continue
		}
		results = append(results, metaResponse(s.prefix, node.Meta))
	}
	return results, nil
}

// Read는 지정된 ID의 파일 핸들을 엽니다.
func (s *VfsService) Read(id uuid.UUID) (*vfs.File, error) {
	return s.vfs.Open(id)
}

// Stat은 지정된 ID의 메타데이터를 조회합니다.
func (s *VfsService) Stat(id uuid.UUID) (types.MetaRes, error) {
	meta, err := s.vfs.Stat(id)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
}

// Create은 새로운 파일을 생성합니다.
func (s *VfsService) Create(name string, file io.Reader) (types.MetaRes, error) {
	meta, err := s.vfs.Create(name, file)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
}

// Mkdir은 새로운 디렉토리를 생성합니다.
func (s *VfsService) Mkdir(name string) (types.MetaRes, error) {
	meta, err := s.vfs.Mkdir(name)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
}

// Write는 파일 내용을 업데이트합니다.
func (s *VfsService) Write(id uuid.UUID, content io.Reader) (types.MetaRes, error) {
	meta, err := s.vfs.Write(id, content)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
}

// Move는 파일 또는 디렉토리를 이동합니다.
func (s *VfsService) Move(id uuid.UUID, dst string, replaceID ...uuid.UUID) (types.MetaRes, error) {
	meta, err := s.vfs.Move(id, dst, replaceID...)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
}

// Copy는 파일 또는 디렉토리를 복사합니다.
func (s *VfsService) Copy(id uuid.UUID, dst string, replaceID ...uuid.UUID) (types.MetaRes, error) {
	meta, err := s.vfs.Copy(id, dst, replaceID...)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
}

// Delete는 파일 또는 디렉토리를 삭제합니다.
func (s *VfsService) Delete(id uuid.UUID) error {
	return s.vfs.Delete(id)
}

// WriteComments는 항목에 대한 설명을 업데이트합니다.
func (s *VfsService) WriteComments(id uuid.UUID, comment string) (types.MetaRes, error) {
	meta, err := s.vfs.WriteComments(id, comment)
	if err != nil {
		return types.MetaRes{}, err
	}
	return metaResponse(s.prefix, meta), nil
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

// NewVfsService는 새로운 VfsService 인스턴스를 생성합니다.
func NewVfsService(fs vfs.VFS, prefix string) *VfsService {
	return &VfsService{prefix: prefix, vfs: fs}
}

func metaResponse(prefix string, meta vfs.Meta) types.MetaRes {
	var resourceURL string
	if meta.IsDir {
		resourceURL = prefix + "?q=" + url.QueryEscape(meta.Path)
	} else {
		resourceURL = path.Join(prefix, meta.ID.String())
	}
	return types.MetaRes{Meta: meta, URL: resourceURL}
}

func mapTreeNodeRes(prefix string, node *vfs.TreeNode) *types.TreeNodeRes {
	if node == nil {
		return nil
	}

	var childrenRes []*types.TreeNodeRes
	if len(node.Children) > 0 {
		childrenRes = make([]*types.TreeNodeRes, len(node.Children))
		for i, child := range node.Children {
			childrenRes[i] = mapTreeNodeRes(prefix, child)
		}
	}

	return &types.TreeNodeRes{
		Meta:     metaResponse(prefix, node.Meta),
		Children: childrenRes,
	}
}
